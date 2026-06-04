package ssh

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PatriceAndoniaina/isolation-manager/src/internal/log"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

func TestMain(m *testing.M) {
	// Silence l'audit pendant les tests.
	log.SetOutput(io.Discard)
	m.Run()
}

// fakeExecutor enregistre les appels et simule ssh-keygen en créant les fichiers
// de clés sur disque, ce qui permet de tester EnsureKey de bout en bout.
type fakeExecutor struct {
	captureCalls [][]string
	interactCall [][]string
	captureErr   error
	interactErr  error
}

func (f *fakeExecutor) Capture(_ context.Context, argv []string) ([]byte, error) {
	f.captureCalls = append(f.captureCalls, argv)
	if f.captureErr != nil {
		return []byte("keygen failed"), f.captureErr
	}
	// Simule ssh-keygen : matérialise la clé privée et publique.
	for i, a := range argv {
		if a == "-f" && i+1 < len(argv) {
			path := argv[i+1]
			_ = os.WriteFile(path, []byte("PRIVATE-KEY-MATERIAL"), 0o600)
			_ = os.WriteFile(path+".pub", []byte("ssh-ed25519 AAAAFAKEKEY isolation-manager\n"), 0o644)
		}
	}
	return nil, nil
}

func (f *fakeExecutor) Interactive(_ context.Context, argv []string, _ Streams) error {
	f.interactCall = append(f.interactCall, argv)
	return f.interactErr
}

func newTestManager(t *testing.T, e Executor) *Manager {
	t.Helper()
	if e == nil {
		e = &fakeExecutor{}
	}
	return New(WithExecutor(e), WithKeysDir(t.TempDir()))
}

func TestEnsureKeyGenerates(t *testing.T) {
	fe := &fakeExecutor{}
	m := newTestManager(t, fe)

	pub, err := m.EnsureKey(context.Background(), "user01")
	if err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}
	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Errorf("public key = %q, want ssh-ed25519 prefix", pub)
	}
	if len(fe.captureCalls) != 1 {
		t.Fatalf("ssh-keygen called %d times, want 1", len(fe.captureCalls))
	}
	argv := strings.Join(fe.captureCalls[0], " ")
	for _, want := range []string{"ssh-keygen", "-t ed25519", "-N "} {
		if !strings.Contains(argv, want) {
			t.Errorf("keygen args missing %q: %s", want, argv)
		}
	}

	// La clé privée doit exister en 0600.
	info, err := os.Stat(m.keyPath("user01"))
	if err != nil {
		t.Fatalf("stat private key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("private key perm = %o, want 600", perm)
	}
}

func TestEnsureKeyIdempotent(t *testing.T) {
	fe := &fakeExecutor{}
	m := newTestManager(t, fe)
	ctx := context.Background()

	if _, err := m.EnsureKey(ctx, "dup"); err != nil {
		t.Fatalf("first EnsureKey: %v", err)
	}
	if _, err := m.EnsureKey(ctx, "dup"); err != nil {
		t.Fatalf("second EnsureKey: %v", err)
	}
	if len(fe.captureCalls) != 1 {
		t.Errorf("ssh-keygen called %d times, want 1 (reuse)", len(fe.captureCalls))
	}
}

func TestEnsureKeyInvalidName(t *testing.T) {
	m := newTestManager(t, nil)
	_, err := m.EnsureKey(context.Background(), "Bad/Name")
	if !apperrors.Is(err, apperrors.ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
}

func TestEnsureKeyFailure(t *testing.T) {
	fe := &fakeExecutor{captureErr: errors.New("boom")}
	m := newTestManager(t, fe)
	if _, err := m.EnsureKey(context.Background(), "fail"); err == nil {
		t.Fatal("EnsureKey = nil, want error")
	}
}

func TestConnectArgsHardening(t *testing.T) {
	m := newTestManager(t, nil)
	c := &container.Container{Name: "hard", SSHPort: 12345}
	argv := m.connectArgs(c, "root")
	joined := strings.Join(argv, " ")

	required := []string{
		"-i " + m.keyPath("hard"),
		"-p 12345",
		"IdentitiesOnly=yes",
		"PasswordAuthentication=no",
		"PubkeyAuthentication=yes",
		"PreferredAuthentications=publickey",
		"ForwardAgent=no",
		"root@127.0.0.1",
	}
	for _, want := range required {
		if !strings.Contains(joined, want) {
			t.Errorf("connect args missing %q: %s", want, joined)
		}
	}
	if argv[0] != "ssh" {
		t.Errorf("argv[0] = %q, want ssh", argv[0])
	}
}

func TestConnect(t *testing.T) {
	fe := &fakeExecutor{}
	m := newTestManager(t, fe)
	ctx := context.Background()
	if _, err := m.EnsureKey(ctx, "conn"); err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}

	c := &container.Container{Name: "conn", SSHPort: 23456}
	err := m.Connect(ctx, ConnectOptions{Container: c})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if len(fe.interactCall) != 1 {
		t.Fatalf("interactive called %d times, want 1", len(fe.interactCall))
	}
	joined := strings.Join(fe.interactCall[0], " ")
	if !strings.Contains(joined, "-p 23456") || !strings.Contains(joined, "root@127.0.0.1") {
		t.Errorf("unexpected ssh argv: %s", joined)
	}
}

func TestConnectDefaultsUser(t *testing.T) {
	fe := &fakeExecutor{}
	m := newTestManager(t, fe)
	ctx := context.Background()
	if _, err := m.EnsureKey(ctx, "duser"); err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}
	c := &container.Container{Name: "duser", SSHPort: 11111}
	if err := m.Connect(ctx, ConnectOptions{Container: c, User: "alice"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if joined := strings.Join(fe.interactCall[0], " "); !strings.Contains(joined, "alice@127.0.0.1") {
		t.Errorf("custom user not honored: %s", joined)
	}
}

func TestConnectMissingKey(t *testing.T) {
	m := newTestManager(t, &fakeExecutor{})
	c := &container.Container{Name: "nokey", SSHPort: 10001}
	if err := m.Connect(context.Background(), ConnectOptions{Container: c}); err == nil {
		t.Fatal("Connect without key = nil, want error")
	}
}

func TestConnectNilContainer(t *testing.T) {
	m := newTestManager(t, &fakeExecutor{})
	if err := m.Connect(context.Background(), ConnectOptions{}); err == nil {
		t.Fatal("Connect nil container = nil, want error")
	}
}

func TestConnectInteractiveError(t *testing.T) {
	fe := &fakeExecutor{interactErr: errors.New("connection refused")}
	m := newTestManager(t, fe)
	ctx := context.Background()
	if _, err := m.EnsureKey(ctx, "err"); err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}
	c := &container.Container{Name: "err", SSHPort: 10002}
	if err := m.Connect(ctx, ConnectOptions{Container: c}); err == nil {
		t.Fatal("Connect = nil, want propagated error")
	}
}

func TestInstallKey(t *testing.T) {
	m := newTestManager(t, &fakeExecutor{})
	ctx := context.Background()
	if _, err := m.EnsureKey(ctx, "inst"); err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}

	rootfs := t.TempDir()
	if err := m.InstallKey("inst", rootfs); err != nil {
		t.Fatalf("InstallKey: %v", err)
	}

	authorized := filepath.Join(rootfs, "root", ".ssh", "authorized_keys")
	data, err := os.ReadFile(authorized)
	if err != nil {
		t.Fatalf("read authorized_keys: %v", err)
	}
	if !strings.HasPrefix(string(data), "ssh-ed25519 ") {
		t.Errorf("authorized_keys content = %q", data)
	}

	info, _ := os.Stat(authorized)
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("authorized_keys perm = %o, want 600", perm)
	}
	dirInfo, _ := os.Stat(filepath.Join(rootfs, "root", ".ssh"))
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf(".ssh perm = %o, want 700", perm)
	}
}

func TestInstallKeyMissing(t *testing.T) {
	m := newTestManager(t, &fakeExecutor{})
	if err := m.InstallKey("ghost", t.TempDir()); err == nil {
		t.Fatal("InstallKey without key = nil, want error")
	}
}

func TestSystemExecutor(t *testing.T) {
	ctx := context.Background()
	e := systemExecutor{}

	out, err := e.Capture(ctx, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("Capture out = %q, want hello", out)
	}
	if _, err := e.Capture(ctx, nil); err == nil {
		t.Error("Capture(nil) should error")
	}

	var buf strings.Builder
	if err := e.Interactive(ctx, []string{"echo", "world"}, Streams{Out: &buf}); err != nil {
		t.Fatalf("Interactive: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "world" {
		t.Errorf("Interactive out = %q, want world", buf.String())
	}
	if err := e.Interactive(ctx, nil, Streams{}); err == nil {
		t.Error("Interactive(nil) should error")
	}
}
