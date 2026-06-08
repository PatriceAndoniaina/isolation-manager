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

func TestExecRunner(t *testing.T) {
	out, err := execRunner{}.Run(context.Background(), []string{"echo", "ok"})
	if err != nil || strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("Run = %q, %v", out, err)
	}
	if _, err := (execRunner{}).Run(context.Background(), nil); err == nil {
		t.Error("Run(nil) should error")
	}
}
