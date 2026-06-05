package nginx

import (
	"strings"
	"testing"
)

func TestFormatBasic(t *testing.T) {
	dirs, err := Parse("http{server{listen 443 ssl;location /{proxy_pass http://x:80;}}}")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got := Format(dirs)
	want := `http {
    server {
        listen 443 ssl;
        location / {
            proxy_pass http://x:80;
        }
    }
}
`
	if got != want {
		t.Errorf("Format =\n%s\nwant\n%s", got, want)
	}
}

func TestFormatRequotesArgs(t *testing.T) {
	// Un argument contenant des espaces doit être ré-entouré de guillemets.
	dirs, _ := Parse(`log_format main "$remote_addr - $request";`)
	got := Format(dirs)
	if !strings.Contains(got, `"$remote_addr - $request"`) {
		t.Errorf("Format should re-quote spaced arg: %s", got)
	}
	// Et la sortie doit se reparser à l'identique.
	re, err := Parse(got)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if re[0].Args[1] != "$remote_addr - $request" {
		t.Errorf("round-trip arg = %q", re[0].Args[1])
	}
}

func TestFormatEmptyBlock(t *testing.T) {
	dirs, _ := Parse("events { }")
	if got := Format(dirs); got != "events {\n}\n" {
		t.Errorf("Format empty block = %q", got)
	}
}

func TestFormatIdempotent(t *testing.T) {
	dirs, _ := Parse(sample)
	once := Format(dirs)
	reparsed, err := Parse(once)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	twice := Format(reparsed)
	if once != twice {
		t.Errorf("Format not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestFormatRoundTripStructure(t *testing.T) {
	dirs, _ := Parse(sample)
	reparsed, err := Parse(Format(dirs))
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !sameTree(dirs, reparsed) {
		t.Error("Parse(Format(x)) differs structurally from x")
	}
}

// sameTree compare deux arbres en ignorant les numéros de ligne.
func sameTree(a, b []*Directive) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || len(a[i].Args) != len(b[i].Args) {
			return false
		}
		for j := range a[i].Args {
			if a[i].Args[j] != b[i].Args[j] {
				return false
			}
		}
		if (a[i].Block == nil) != (b[i].Block == nil) {
			return false
		}
		if !sameTree(a[i].Block, b[i].Block) {
			return false
		}
	}
	return true
}
