package deploy

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// fakeCommander enregistre les commandes et renvoie une réponse programmée.
type fakeCommander struct {
	captures    []Cmd
	interacts   []Cmd
	respond     func(Cmd) ([]byte, error)
	interactErr error
}

func (f *fakeCommander) Capture(_ context.Context, c Cmd) ([]byte, error) {
	f.captures = append(f.captures, c)
	if f.respond != nil {
		return f.respond(c)
	}
	return nil, nil
}

func (f *fakeCommander) Interactive(_ context.Context, c Cmd, _ Streams) error {
	f.interacts = append(f.interacts, c)
	return f.interactErr
}

// isUname indique si la commande est le ssh de détection uname.
func isUname(c Cmd) bool {
	return c.Name == "ssh" && len(c.Args) > 0 && strings.Contains(c.Args[len(c.Args)-1], "uname")
}

// findCapture renvoie la première commande capturée satisfaisant pred.
func findCapture(f *fakeCommander, pred func(Cmd) bool) (Cmd, bool) {
	for _, c := range f.captures {
		if pred(c) {
			return c, true
		}
	}
	return Cmd{}, false
}

func linuxAmd64() func(Cmd) ([]byte, error) {
	return func(c Cmd) ([]byte, error) {
		if isUname(c) {
			return []byte("Linux x86_64\n"), nil
		}
		return nil, nil
	}
}

func opts() Options {
	return Options{
		Target:      Target{Host: "srv", User: "root"},
		RemotePath:  "/usr/local/bin/isolation-manager",
		Source:      ".",
		InstallDeps: true,
	}
}

func TestDeployHappyPath(t *testing.T) {
	fc := &fakeCommander{respond: linuxAmd64()}
	d := New(WithCommander(fc))

	if err := d.Deploy(context.Background(), io.Discard, opts()); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	// Compilation croisée linux/amd64.
	build, ok := findCapture(fc, func(c Cmd) bool { return c.Name == "go" })
	if !ok {
		t.Fatal("go build not invoked")
	}
	env := strings.Join(build.Env, " ")
	if !strings.Contains(env, "GOOS=linux") || !strings.Contains(env, "GOARCH=amd64") {
		t.Errorf("build env = %v, want GOOS=linux GOARCH=amd64", build.Env)
	}

	// Transfert rsync.
	rsync, ok := findCapture(fc, func(c Cmd) bool { return c.Name == "rsync" })
	if !ok {
		t.Fatal("rsync not invoked")
	}
	if joined := strings.Join(rsync.Args, " "); !strings.Contains(joined, "root@srv:") {
		t.Errorf("rsync dest missing: %s", joined)
	}

	// Installation + dépendances (ssh contenant 'install' et 'systemd').
	if _, ok := findCapture(fc, func(c Cmd) bool {
		return c.Name == "ssh" && strings.Contains(c.Args[len(c.Args)-1], "install")
	}); !ok {
		t.Error("install step not invoked")
	}
	if _, ok := findCapture(fc, func(c Cmd) bool {
		return c.Name == "ssh" && strings.Contains(c.Args[len(c.Args)-1], "systemd-container")
	}); !ok {
		t.Error("dependency install not invoked")
	}
}

func TestDeployNonLinux(t *testing.T) {
	fc := &fakeCommander{respond: func(c Cmd) ([]byte, error) {
		if isUname(c) {
			return []byte("Darwin arm64\n"), nil
		}
		return nil, nil
	}}
	if err := New(WithCommander(fc)).Deploy(context.Background(), io.Discard, opts()); err == nil {
		t.Fatal("Deploy on Darwin = nil, want error")
	}
}

func TestDeployUnknownArch(t *testing.T) {
	fc := &fakeCommander{respond: func(c Cmd) ([]byte, error) {
		if isUname(c) {
			return []byte("Linux riscv64\n"), nil
		}
		return nil, nil
	}}
	if err := New(WithCommander(fc)).Deploy(context.Background(), io.Discard, opts()); err == nil {
		t.Fatal("Deploy on riscv64 = nil, want error")
	}
}

func TestDeployDetectError(t *testing.T) {
	fc := &fakeCommander{respond: func(c Cmd) ([]byte, error) {
		if isUname(c) {
			return []byte("connection refused"), errors.New("ssh failed")
		}
		return nil, nil
	}}
	if err := New(WithCommander(fc)).Deploy(context.Background(), io.Discard, opts()); err == nil {
		t.Fatal("Deploy with ssh error = nil, want error")
	}
}

func TestDeployBuildError(t *testing.T) {
	fc := &fakeCommander{respond: func(c Cmd) ([]byte, error) {
		if isUname(c) {
			return []byte("Linux x86_64\n"), nil
		}
		if c.Name == "go" {
			return []byte("build failed"), errors.New("exit 1")
		}
		return nil, nil
	}}
	if err := New(WithCommander(fc)).Deploy(context.Background(), io.Discard, opts()); err == nil {
		t.Fatal("Deploy with build error = nil, want error")
	}
}

func TestDeployRsyncError(t *testing.T) {
	fc := &fakeCommander{respond: func(c Cmd) ([]byte, error) {
		if isUname(c) {
			return []byte("Linux aarch64\n"), nil
		}
		if c.Name == "rsync" {
			return []byte("rsync error"), errors.New("exit 23")
		}
		return nil, nil
	}}
	if err := New(WithCommander(fc)).Deploy(context.Background(), io.Discard, opts()); err == nil {
		t.Fatal("Deploy with rsync error = nil, want error")
	}
}

func TestDeployVerifyOnly(t *testing.T) {
	// InstallDeps=false : verifyDeps avertit mais ne fait pas échouer le deploy.
	fc := &fakeCommander{respond: func(c Cmd) ([]byte, error) {
		if isUname(c) {
			return []byte("Linux x86_64\n"), nil
		}
		// La vérification systemd-nspawn échoue → avertissement.
		if c.Name == "ssh" && strings.Contains(c.Args[len(c.Args)-1], "command -v systemd-nspawn") {
			return nil, errors.New("absent")
		}
		return nil, nil
	}}
	o := opts()
	o.InstallDeps = false
	var buf strings.Builder
	if err := New(WithCommander(fc)).Deploy(context.Background(), &buf, o); err != nil {
		t.Fatalf("Deploy = %v, want nil (warning only)", err)
	}
	if !strings.Contains(buf.String(), "absent") {
		t.Errorf("expected warning about missing systemd-nspawn, got: %s", buf.String())
	}
}

func TestDeployValidate(t *testing.T) {
	d := New(WithCommander(&fakeCommander{}))
	tests := []Options{
		{Target: Target{User: "root"}, RemotePath: "/x"}, // host manquant
		{Target: Target{Host: "srv"}, RemotePath: "/x"},  // user manquant
		{Target: Target{Host: "srv", User: "root"}},      // remote-path manquant
	}
	for _, o := range tests {
		if err := d.Deploy(context.Background(), io.Discard, o); err == nil {
			t.Errorf("Deploy(%+v) = nil, want validation error", o)
		}
	}
}

func TestExec(t *testing.T) {
	fc := &fakeCommander{}
	d := New(WithCommander(fc))
	err := d.Exec(context.Background(),
		Target{Host: "srv", User: "root", Port: 2222, Key: "/k"},
		"/usr/local/bin/isolation-manager",
		[]string{"create", "user01", "--memory", "1024"},
		Streams{})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(fc.interacts) != 1 {
		t.Fatalf("interactive called %d times, want 1", len(fc.interacts))
	}
	argv := strings.Join(fc.interacts[0].Args, " ")
	for _, want := range []string{"-t", "-p 2222", "-i /k", "root@srv",
		"sudo /usr/local/bin/isolation-manager", "'create'", "'user01'"} {
		if !strings.Contains(argv, want) {
			t.Errorf("exec argv missing %q: %s", want, argv)
		}
	}
}

func TestExecNoHost(t *testing.T) {
	d := New(WithCommander(&fakeCommander{}))
	if err := d.Exec(context.Background(), Target{}, "/bin/x", []string{"list"}, Streams{}); err == nil {
		t.Fatal("Exec without host = nil, want error")
	}
}

func TestExecInteractiveError(t *testing.T) {
	fc := &fakeCommander{interactErr: errors.New("ssh closed")}
	d := New(WithCommander(fc))
	if err := d.Exec(context.Background(), Target{Host: "srv", User: "root"}, "/bin/x", nil, Streams{}); err == nil {
		t.Fatal("Exec = nil, want propagated error")
	}
}

func TestMapArch(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"x86_64", "amd64", false},
		{"amd64", "amd64", false},
		{"aarch64", "arm64", false},
		{"arm64", "arm64", false},
		{"riscv64", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		got, err := mapArch(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("mapArch(%q) = nil error, want error", tt.in)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("mapArch(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
		}
	}
}

func TestSSHOpts(t *testing.T) {
	got := strings.Join(sshOpts(Target{Port: 2222, Key: "/k"}), " ")
	for _, want := range []string{"StrictHostKeyChecking=accept-new", "-p 2222", "-i /k"} {
		if !strings.Contains(got, want) {
			t.Errorf("sshOpts missing %q: %s", want, got)
		}
	}
	// Sans port ni clé : pas de -p ni -i.
	bare := strings.Join(sshOpts(Target{}), " ")
	if strings.Contains(bare, "-p ") || strings.Contains(bare, "-i ") {
		t.Errorf("bare sshOpts should omit -p/-i: %s", bare)
	}
}

func TestShellQuote(t *testing.T) {
	got := shellQuote([]string{"create", "a b", "it's"})
	want := []string{"'create'", "'a b'", `'it'\''s'`}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("shellQuote = %v, want %v", got, want)
	}
}

func TestSystemCommander(t *testing.T) {
	ctx := context.Background()
	sc := systemCommander{}

	out, err := sc.Capture(ctx, Cmd{Name: "echo", Args: []string{"hi"}})
	if err != nil || strings.TrimSpace(string(out)) != "hi" {
		t.Fatalf("Capture = %q, %v", out, err)
	}
	if _, err := sc.Capture(ctx, Cmd{}); err == nil {
		t.Error("Capture(empty) should error")
	}

	var buf strings.Builder
	if err := sc.Interactive(ctx, Cmd{Name: "echo", Args: []string{"yo"}}, Streams{Out: &buf}); err != nil {
		t.Fatalf("Interactive: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "yo" {
		t.Errorf("Interactive out = %q", buf.String())
	}
	if err := sc.Interactive(ctx, Cmd{}, Streams{}); err == nil {
		t.Error("Interactive(empty) should error")
	}
}
