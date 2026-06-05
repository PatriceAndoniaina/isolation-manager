package nginx

import "testing"

const sample = `
http {
    limit_req_zone $binary_remote_addr zone=z:10m rate=10r/s;
    server {
        listen 443 ssl;
        server_name a.example.com;
        location / {
            proxy_pass http://10.0.0.2:8080;
        }
    }
    server {
        listen 80;
        server_name b.example.com;
    }
}
`

func TestGetAndValue(t *testing.T) {
	dirs, err := Parse(sample)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Chaînage : http > server > listen.
	listen := dirs[0].Get("server").Get("listen")
	if listen == nil {
		t.Fatal("listen not found via Get chain")
	}
	if got := listen.Value(); got != "443 ssl" {
		t.Errorf("Value() = %q, want %q", got, "443 ssl")
	}
	// Get sur récepteur nil reste sûr.
	if dirs[0].Get("absent").Get("x") != nil {
		t.Error("Get chain on missing directive should stay nil")
	}
	if (*Directive)(nil).Value() != "" {
		t.Error("Value() on nil should be empty")
	}
}

func TestFindAndFindAll(t *testing.T) {
	dirs, _ := Parse(sample)
	http := dirs[0]

	// Find : enfants directs uniquement.
	if n := len(Find(http.Block, "server")); n != 2 {
		t.Errorf("Find server = %d, want 2", n)
	}
	if n := len(Find(dirs, "server")); n != 0 {
		t.Errorf("Find server at top-level = %d, want 0", n)
	}

	// FindAll : récursif sur tout l'arbre.
	if n := len(FindAll(dirs, "server")); n != 2 {
		t.Errorf("FindAll server = %d, want 2", n)
	}
	if n := len(FindAll(dirs, "proxy_pass")); n != 1 {
		t.Errorf("FindAll proxy_pass = %d, want 1", n)
	}
	if n := len(FindAll(dirs, "listen")); n != 2 {
		t.Errorf("FindAll listen = %d, want 2", n)
	}
}

func TestSelect(t *testing.T) {
	dirs, _ := Parse(sample)

	listens := Select(dirs, "http", "server", "listen")
	if len(listens) != 2 {
		t.Fatalf("Select http>server>listen = %d, want 2", len(listens))
	}
	if listens[0].Value() != "443 ssl" || listens[1].Value() != "80" {
		t.Errorf("listen values = %q, %q", listens[0].Value(), listens[1].Value())
	}

	pp := Select(dirs, "http", "server", "location", "proxy_pass")
	if len(pp) != 1 || pp[0].Args[0] != "http://10.0.0.2:8080" {
		t.Errorf("Select proxy_pass = %+v", pp)
	}

	if Select(dirs) != nil {
		t.Error("Select with empty path should be nil")
	}
	if got := Select(dirs, "http"); len(got) != 1 {
		t.Errorf("Select single = %d, want 1", len(got))
	}
	if got := Select(dirs, "http", "absent"); got != nil {
		t.Errorf("Select missing leaf = %+v, want nil", got)
	}
}

func TestDirectiveLine(t *testing.T) {
	dirs, _ := Parse(sample)
	// http est sur la ligne 2 (la ligne 1 est vide).
	if dirs[0].Line != 2 {
		t.Errorf("http.Line = %d, want 2", dirs[0].Line)
	}
	pp := FindAll(dirs, "proxy_pass")[0]
	if pp.Line != 8 {
		t.Errorf("proxy_pass.Line = %d, want 8", pp.Line)
	}
}
