package nspawn

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/PatriceAndoniaina/isolation-manager/src/internal/log"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

func TestMain(m *testing.M) {
	// Silence l'audit pendant les tests pour garder une sortie propre.
	log.SetOutput(io.Discard)
	m.Run()
}

// fakeRunner enregistre les commandes reçues et renvoie une réponse programmée.
type fakeRunner struct {
	calls [][]string
	out   []byte
	err   error
}

func (f *fakeRunner) Run(_ context.Context, argv []string) ([]byte, error) {
	f.calls = append(f.calls, argv)
	return f.out, f.err
}

// lastCall renvoie la dernière commande exécutée.
func (f *fakeRunner) lastCall() []string {
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

// newTestManager construit un Manager isolé : Runner mocké, dossiers temporaires.
func newTestManager(t *testing.T, r Runner) *Manager {
	t.Helper()
	if r == nil {
		r = &fakeRunner{}
	}
	return New(
		WithRunner(r),
		WithStateDir(t.TempDir()),
		WithMachinesDir(t.TempDir()),
	)
}

func TestCreate(t *testing.T) {
	m := newTestManager(t, nil)
	ctx := context.Background()

	c, err := m.Create(ctx, container.CreateOptions{Name: "user01"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.State != container.StateCreated {
		t.Errorf("state = %q, want created", c.State)
	}
	if c.SSHPort < 10001 {
		t.Errorf("ssh port = %d, want > 10000", c.SSHPort)
	}
	if !m.store.exists("user01") {
		t.Error("metadata not persisted")
	}
}

func TestCreateInvalidName(t *testing.T) {
	m := newTestManager(t, nil)
	_, err := m.Create(context.Background(), container.CreateOptions{Name: "Bad/Name"})
	if !apperrors.Is(err, apperrors.ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
}

func TestCreateDuplicate(t *testing.T) {
	m := newTestManager(t, nil)
	ctx := context.Background()
	if _, err := m.Create(ctx, container.CreateOptions{Name: "dup"}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := m.Create(ctx, container.CreateOptions{Name: "dup"})
	if !apperrors.Is(err, apperrors.ErrAlreadyExists) {
		t.Fatalf("err = %v, want ErrAlreadyExists", err)
	}
}

func TestCreateAllocatesDistinctPorts(t *testing.T) {
	m := newTestManager(t, nil)
	ctx := context.Background()
	a, err := m.Create(ctx, container.CreateOptions{Name: "alpha"})
	if err != nil {
		t.Fatalf("Create alpha: %v", err)
	}
	b, err := m.Create(ctx, container.CreateOptions{Name: "beta"})
	if err != nil {
		t.Fatalf("Create beta: %v", err)
	}
	if a.SSHPort == b.SSHPort {
		t.Fatalf("ports collide: %d", a.SSHPort)
	}
}

func TestStartAppliesSecurity(t *testing.T) {
	fr := &fakeRunner{}
	m := newTestManager(t, fr)
	ctx := context.Background()
	if _, err := m.Create(ctx, container.CreateOptions{Name: "sec"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Start(ctx, "sec"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	argv := strings.Join(fr.lastCall(), " ")
	for _, want := range []string{"--read-only", "--machine=sec", "MemoryMax", "systemd-nspawn"} {
		if !strings.Contains(argv, want) {
			t.Errorf("start args missing %q: %s", want, argv)
		}
	}
	if strings.Contains(argv, "--privileged") {
		t.Errorf("start args must not contain --privileged: %s", argv)
	}

	got, err := m.Get(ctx, "sec")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	// isRunning interroge machinectl ; le fake renvoie "" → considéré arrêté.
	if got.State != container.StateStopped {
		t.Errorf("state = %q, want stopped", got.State)
	}
}

func TestStartNotFound(t *testing.T) {
	m := newTestManager(t, nil)
	err := m.Start(context.Background(), "ghost")
	if !apperrors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGuardRejectsPrivileged(t *testing.T) {
	if err := guardArgs([]string{"systemd-nspawn", "--privileged"}); !apperrors.Is(err, apperrors.ErrSecurityViolation) {
		t.Fatalf("err = %v, want ErrSecurityViolation", err)
	}
	if err := guardArgs([]string{"systemd-nspawn", "--capability=CAP_SYS_ADMIN"}); !apperrors.Is(err, apperrors.ErrSecurityViolation) {
		t.Fatalf("err = %v, want ErrSecurityViolation", err)
	}
	if err := guardArgs([]string{"systemd-nspawn", "--read-only"}); err != nil {
		t.Fatalf("safe args rejected: %v", err)
	}
}

func TestExecGuardBlocksRun(t *testing.T) {
	fr := &fakeRunner{}
	m := newTestManager(t, fr)
	_, err := m.exec(context.Background(), "test", "x", []string{"systemd-nspawn", "--privileged"})
	if !apperrors.Is(err, apperrors.ErrSecurityViolation) {
		t.Fatalf("err = %v, want ErrSecurityViolation", err)
	}
	if len(fr.calls) != 0 {
		t.Errorf("runner invoked despite guard: %v", fr.calls)
	}
}

func TestStopUpdatesState(t *testing.T) {
	m := newTestManager(t, &fakeRunner{})
	ctx := context.Background()
	if _, err := m.Create(ctx, container.CreateOptions{Name: "st"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Start(ctx, "st"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop(ctx, "st"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	c, _ := m.store.load("st")
	if c.State != container.StateStopped {
		t.Errorf("state = %q, want stopped", c.State)
	}
}

func TestDestroyRemovesState(t *testing.T) {
	m := newTestManager(t, &fakeRunner{})
	ctx := context.Background()
	if _, err := m.Create(ctx, container.CreateOptions{Name: "del"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Destroy(ctx, "del"); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if m.store.exists("del") {
		t.Error("metadata still present after destroy")
	}
	if _, err := m.Get(ctx, "del"); !apperrors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("Get err = %v, want ErrNotFound", err)
	}
}

func TestDestroyNotFound(t *testing.T) {
	m := newTestManager(t, &fakeRunner{})
	err := m.Destroy(context.Background(), "ghost")
	if !apperrors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListReturnsCreated(t *testing.T) {
	m := newTestManager(t, nil)
	ctx := context.Background()
	for _, n := range []string{"one", "two", "three"} {
		if _, err := m.Create(ctx, container.CreateOptions{Name: n}); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}
	list, err := m.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len = %d, want 3", len(list))
	}
}

func TestGetRunningRefresh(t *testing.T) {
	// Le fake annonce "running" : Get doit refléter l'état réel.
	fr := &fakeRunner{out: []byte("running\n")}
	m := newTestManager(t, fr)
	ctx := context.Background()
	if _, err := m.Create(ctx, container.CreateOptions{Name: "live"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	c, err := m.Get(ctx, "live")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if c.State != container.StateRunning {
		t.Errorf("state = %q, want running", c.State)
	}
}

// scriptedRunner échoue sur les commandes contenant failOn, réussit sinon.
type scriptedRunner struct {
	failOn string
	calls  [][]string
}

func (s *scriptedRunner) Run(_ context.Context, argv []string) ([]byte, error) {
	s.calls = append(s.calls, argv)
	for _, a := range argv {
		if a == s.failOn {
			return nil, errors.New("command failed")
		}
	}
	return nil, nil
}

func TestStopFallbackToTerminate(t *testing.T) {
	// poweroff échoue → repli sur terminate, l'état doit passer à stopped.
	sr := &scriptedRunner{failOn: "poweroff"}
	m := newTestManager(t, sr)
	ctx := context.Background()
	if _, err := m.Create(ctx, container.CreateOptions{Name: "fb"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := m.Stop(ctx, "fb"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	last := strings.Join(sr.calls[len(sr.calls)-1], " ")
	if !strings.Contains(last, "terminate") {
		t.Errorf("expected terminate fallback, got %s", last)
	}
	c, _ := m.store.load("fb")
	if c.State != container.StateStopped {
		t.Errorf("state = %q, want stopped", c.State)
	}
}

func TestExecRunner(t *testing.T) {
	out, err := execRunner{}.Run(context.Background(), []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("out = %q, want hello", out)
	}
	if _, err := (execRunner{}).Run(context.Background(), nil); err == nil {
		t.Error("empty argv should error")
	}
}

func TestExecMapsTimeout(t *testing.T) {
	// Un ctx dont la deadline est dépassée doit produire ErrTimeout.
	fr := &fakeRunner{err: errors.New("signal: killed")}
	m := newTestManager(t, fr)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err := m.exec(ctx, "test", "x", []string{"machinectl", "show", "x"})
	if !apperrors.Is(err, apperrors.ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}
