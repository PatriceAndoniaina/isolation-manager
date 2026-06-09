package nginx

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls [][]string
	out   []byte
	err   error
}

func (f *fakeRunner) Run(_ context.Context, argv []string) ([]byte, error) {
	f.calls = append(f.calls, argv)
	return f.out, f.err
}

func TestCheckSuccess(t *testing.T) {
	fr := &fakeRunner{out: []byte("nginx: configuration file ok\n")}
	out, err := NewTester(WithRunner(fr)).Check(context.Background(), "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !strings.Contains(string(out), "ok") {
		t.Errorf("out = %q", out)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(fr.calls))
	}
	argv := strings.Join(fr.calls[0], " ")
	if argv != "nginx -t" {
		t.Errorf("argv = %q, want \"nginx -t\"", argv)
	}
}

func TestCheckWithFile(t *testing.T) {
	fr := &fakeRunner{}
	if _, err := NewTester(WithRunner(fr)).Check(context.Background(), "/etc/nginx/nginx.conf"); err != nil {
		t.Fatalf("Check: %v", err)
	}
	argv := strings.Join(fr.calls[0], " ")
	if argv != "nginx -t -c /etc/nginx/nginx.conf" {
		t.Errorf("argv = %q", argv)
	}
}

func TestCheckFailure(t *testing.T) {
	fr := &fakeRunner{out: []byte("nginx: [emerg] unexpected }\n"), err: errors.New("exit 1")}
	out, err := NewTester(WithRunner(fr)).Check(context.Background(), "")
	if err == nil {
		t.Fatal("Check = nil, want error on invalid config")
	}
	// La sortie de nginx doit être conservée pour diagnostic.
	if !strings.Contains(string(out), "emerg") {
		t.Errorf("output not preserved: %q", out)
	}
}

// has renvoie un détecteur de binaire simulant la présence des noms donnés.
func has(names ...string) func(string) bool {
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	return func(n string) bool { return set[n] }
}

func TestReloadAdaptsToOS(t *testing.T) {
	tests := []struct {
		name string
		goos string
		look func(string) bool
		want string
	}{
		{"linux systemd", "linux", has("systemctl"), "systemctl reload nginx"},
		{"linux sysv", "linux", has("service"), "service nginx reload"},
		{"linux systemd prioritaire", "linux", has("systemctl", "service"), "systemctl reload nginx"},
		{"linux sans gestionnaire", "linux", has(), "nginx -s reload"},
		{"darwin", "darwin", has("systemctl"), "nginx -s reload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeRunner{}
			te := NewTester(WithRunner(fr), WithGOOS(tt.goos), WithLooker(tt.look))
			if _, err := te.Reload(context.Background()); err != nil {
				t.Fatalf("Reload: %v", err)
			}
			if got := strings.Join(fr.calls[0], " "); got != tt.want {
				t.Errorf("reload cmd = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRestartViaServiceManager(t *testing.T) {
	tests := []struct {
		name string
		look func(string) bool
		want string
	}{
		{"linux systemd", has("systemctl"), "systemctl restart nginx"},
		{"linux sysv", has("service"), "service nginx restart"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeRunner{}
			te := NewTester(WithRunner(fr), WithGOOS("linux"), WithLooker(tt.look))
			if _, err := te.Restart(context.Background()); err != nil {
				t.Fatalf("Restart: %v", err)
			}
			if len(fr.calls) != 1 {
				t.Fatalf("calls = %d, want 1 (single service command)", len(fr.calls))
			}
			if got := strings.Join(fr.calls[0], " "); got != tt.want {
				t.Errorf("restart cmd = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRestartFallbackStopThenStart(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			fr := &fakeRunner{}
			// Aucun gestionnaire de services → repli stop+start.
			te := NewTester(WithRunner(fr), WithGOOS(goos), WithLooker(has()))
			if _, err := te.Restart(context.Background()); err != nil {
				t.Fatalf("Restart: %v", err)
			}
			if len(fr.calls) != 2 {
				t.Fatalf("calls = %d, want 2 (stop puis start)", len(fr.calls))
			}
			if got := strings.Join(fr.calls[0], " "); got != "nginx -s stop" {
				t.Errorf("call[0] = %q, want \"nginx -s stop\"", got)
			}
			if got := strings.Join(fr.calls[1], " "); got != "nginx" {
				t.Errorf("call[1] = %q, want \"nginx\"", got)
			}
		})
	}
}

func TestStatusViaServiceManager(t *testing.T) {
	tests := []struct {
		name string
		look func(string) bool
		want string
	}{
		{"linux systemd", has("systemctl"), "systemctl status nginx"},
		{"linux sysv", has("service"), "service nginx status"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := &fakeRunner{out: []byte("active (running)")}
			te := NewTester(WithRunner(fr), WithGOOS("linux"), WithLooker(tt.look))
			out, err := te.Status(context.Background())
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if !strings.Contains(string(out), "active") {
				t.Errorf("out = %q", out)
			}
			if got := strings.Join(fr.calls[0], " "); got != tt.want {
				t.Errorf("status cmd = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusInactiveIsNotWrapped(t *testing.T) {
	// systemctl renvoie un code non nul si nginx est inactif : l'erreur brute
	// est transmise (pas de "... a échoué"), la sortie est conservée.
	fr := &fakeRunner{out: []byte("inactive (dead)"), err: errors.New("exit status 3")}
	te := NewTester(WithRunner(fr), WithGOOS("linux"), WithLooker(has("systemctl")))
	out, err := te.Status(context.Background())
	if err == nil {
		t.Fatal("Status = nil err, want raw exit error")
	}
	if strings.Contains(err.Error(), "a échoué") {
		t.Errorf("error should be raw, got %q", err)
	}
	if !strings.Contains(string(out), "inactive") {
		t.Errorf("status output not preserved: %q", out)
	}
}

func TestStatusNoServiceManager(t *testing.T) {
	fr := &fakeRunner{}
	te := NewTester(WithRunner(fr), WithGOOS("darwin"), WithLooker(has()))
	if _, err := te.Status(context.Background()); err == nil {
		t.Fatal("Status = nil, want error on host without service manager")
	}
	if len(fr.calls) != 0 {
		t.Error("runner must not be called when no service manager")
	}
}

func TestReloadFailure(t *testing.T) {
	fr := &fakeRunner{out: []byte("nginx: [error] ... failed"), err: errors.New("exit 1")}
	te := NewTester(WithRunner(fr), WithGOOS("linux"), WithLooker(has("systemctl")))
	if _, err := te.Reload(context.Background()); err == nil {
		t.Fatal("Reload = nil, want error")
	}
}

func TestExecRunner(t *testing.T) {
	out, err := execRunner{}.Run(context.Background(), []string{"echo", "ok"})
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("Run = %q, %v", out, err)
	}
	if _, err := (execRunner{}).Run(context.Background(), nil); err == nil {
		t.Error("Run(nil) should error")
	}
}
