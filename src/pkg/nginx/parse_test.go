package nginx

import (
	"strings"
	"testing"
)

func TestParseNestedBlocks(t *testing.T) {
	const cfg = `
http {
    # commentaire
    server {
        listen 443 ssl;
        server_name example.com;
        location / {
            proxy_pass http://10.0.0.2:8080;
        }
    }
}
`
	dirs, err := Parse(cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(dirs) != 1 || dirs[0].Name != "http" {
		t.Fatalf("top-level = %+v, want single http block", dirs)
	}
	http := dirs[0]
	if len(http.Block) != 1 || http.Block[0].Name != "server" {
		t.Fatalf("http block = %+v, want single server", http.Block)
	}
	server := http.Block[0]

	var listen *Directive
	for _, d := range server.Block {
		if d.Name == "listen" {
			listen = d
		}
	}
	if listen == nil {
		t.Fatal("listen directive not found")
	}
	if len(listen.Args) != 2 || listen.Args[0] != "443" || listen.Args[1] != "ssl" {
		t.Errorf("listen args = %v, want [443 ssl]", listen.Args)
	}
}

func TestParseQuotedStrings(t *testing.T) {
	const cfg = `log_format main "$remote_addr - $request";`
	dirs, err := Parse(cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(dirs) != 1 {
		t.Fatalf("got %d directives, want 1", len(dirs))
	}
	if got := dirs[0].Args[1]; got != "$remote_addr - $request" {
		t.Errorf("quoted arg = %q", got)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
	}{
		{"unexpected close brace", "server { } }"},
		{"missing close brace", "server { listen 80;"},
		{"unterminated string", `log_format "unclosed;`},
		{"unterminated directive", "listen 80"},
		{"brace inside directive", "listen 80 } ;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse(tt.cfg); err == nil {
				t.Fatalf("Parse(%q) = nil, want error", tt.cfg)
			}
		})
	}
}

func TestParseEmpty(t *testing.T) {
	dirs, err := Parse("   \n # only a comment \n")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(dirs) != 0 {
		t.Errorf("got %d directives, want 0", len(dirs))
	}
}

func TestWalk(t *testing.T) {
	dirs, _ := Parse("http { server { listen 80; } }")
	var names []string
	Walk(dirs, func(d *Directive) { names = append(names, d.Name) })
	if got := strings.Join(names, ","); got != "http,server,listen" {
		t.Errorf("walk order = %q, want http,server,listen", got)
	}
}
