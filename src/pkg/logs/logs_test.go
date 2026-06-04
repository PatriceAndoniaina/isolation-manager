package logs

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/PatriceAndoniaina/isolation-manager/src/internal/log"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

func TestMain(m *testing.M) {
	// Silence l'audit pendant les tests.
	log.SetOutput(io.Discard)
	m.Run()
}

// fakeJournal enregistre les appels et renvoie une réponse programmée.
type fakeJournal struct {
	captureCalls [][]string
	streamCalls  [][]string
	out          []byte
	captureErr   error
	streamErr    error
}

func (f *fakeJournal) Capture(_ context.Context, argv []string) ([]byte, error) {
	f.captureCalls = append(f.captureCalls, argv)
	return f.out, f.captureErr
}

func (f *fakeJournal) Stream(_ context.Context, argv []string, out io.Writer) error {
	f.streamCalls = append(f.streamCalls, argv)
	if f.out != nil {
		_, _ = out.Write(f.out)
	}
	return f.streamErr
}

func TestFetchCapture(t *testing.T) {
	fj := &fakeJournal{out: []byte("line one\nline two\n")}
	m := New(WithJournal(fj))

	var buf strings.Builder
	err := m.Fetch(context.Background(), &buf, Options{Name: "user01", Lines: 50})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if buf.String() != "line one\nline two\n" {
		t.Errorf("output = %q", buf.String())
	}
	if len(fj.captureCalls) != 1 {
		t.Fatalf("Capture called %d times, want 1", len(fj.captureCalls))
	}
	argv := strings.Join(fj.captureCalls[0], " ")
	for _, want := range []string{"journalctl", "-u isolation-manager-user01.service", "--no-pager", "-n 50"} {
		if !strings.Contains(argv, want) {
			t.Errorf("journalctl args missing %q: %s", want, argv)
		}
	}
}

func TestFetchSince(t *testing.T) {
	fj := &fakeJournal{}
	m := New(WithJournal(fj))
	if err := m.Fetch(context.Background(), io.Discard, Options{Name: "xx", Since: "yesterday"}); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if argv := strings.Join(fj.captureCalls[0], " "); !strings.Contains(argv, "--since yesterday") {
		t.Errorf("--since not passed: %s", argv)
	}
}

func TestFetchFollow(t *testing.T) {
	fj := &fakeJournal{out: []byte("streamed\n")}
	m := New(WithJournal(fj))

	var buf strings.Builder
	err := m.Fetch(context.Background(), &buf, Options{Name: "live", Follow: true})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(fj.streamCalls) != 1 || len(fj.captureCalls) != 0 {
		t.Fatalf("follow should Stream, not Capture: stream=%d capture=%d",
			len(fj.streamCalls), len(fj.captureCalls))
	}
	if buf.String() != "streamed\n" {
		t.Errorf("output = %q", buf.String())
	}
	if argv := strings.Join(fj.streamCalls[0], " "); !strings.Contains(argv, "-f") {
		t.Errorf("follow flag missing: %s", argv)
	}
}

func TestFetchInvalidName(t *testing.T) {
	m := New(WithJournal(&fakeJournal{}))
	err := m.Fetch(context.Background(), io.Discard, Options{Name: "Bad/Name"})
	if !apperrors.Is(err, apperrors.ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
}

func TestFetchNegativeLines(t *testing.T) {
	m := New(WithJournal(&fakeJournal{}))
	if err := m.Fetch(context.Background(), io.Discard, Options{Name: "xx", Lines: -1}); err == nil {
		t.Fatal("Fetch with negative lines = nil, want error")
	}
}

func TestFetchCaptureError(t *testing.T) {
	fj := &fakeJournal{captureErr: errors.New("journalctl boom"), out: []byte("no entries")}
	m := New(WithJournal(fj))
	if err := m.Fetch(context.Background(), io.Discard, Options{Name: "xx"}); err == nil {
		t.Fatal("Fetch = nil, want propagated error")
	}
}

func TestFetchStreamError(t *testing.T) {
	fj := &fakeJournal{streamErr: errors.New("stream broke")}
	m := New(WithJournal(fj))
	if err := m.Fetch(context.Background(), io.Discard, Options{Name: "xx", Follow: true}); err == nil {
		t.Fatal("Fetch = nil, want propagated error")
	}
}

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want []string
	}{
		{
			name: "defaults",
			opts: Options{Name: "a"},
			want: []string{"journalctl", "-u", "unit.service", "--no-pager"},
		},
		{
			name: "all options",
			opts: Options{Name: "a", Lines: 10, Since: "1h ago", Follow: true},
			want: []string{"journalctl", "-u", "unit.service", "--no-pager", "-n", "10", "--since", "1h ago", "-f"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildArgs("unit.service", tt.opts)
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Errorf("buildArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSystemJournal(t *testing.T) {
	ctx := context.Background()
	j := systemJournal{}

	out, err := j.Capture(ctx, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("Capture out = %q, want hello", out)
	}
	if _, err := j.Capture(ctx, nil); err == nil {
		t.Error("Capture(nil) should error")
	}

	var buf strings.Builder
	if err := j.Stream(ctx, []string{"echo", "world"}, &buf); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "world" {
		t.Errorf("Stream out = %q, want world", buf.String())
	}
	if err := j.Stream(ctx, nil, io.Discard); err == nil {
		t.Error("Stream(nil) should error")
	}
}
