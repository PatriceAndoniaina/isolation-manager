package nginx

import (
	"testing"

	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// compliant est une configuration conforme aux règles de sécurité.
const compliant = `
limit_req_zone $binary_remote_addr zone=z:10m rate=10r/s;
server {
    listen 443 ssl;
    ssl_certificate /etc/ssl/x.crt;
    client_max_body_size 10m;
    location / {
        limit_req zone=z burst=20 nodelay;
        proxy_pass http://10.0.0.2:8080;
    }
}
`

func mustParse(t *testing.T, cfg string) []*Directive {
	t.Helper()
	dirs, err := Parse(cfg)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return dirs
}

func TestValidateCompliant(t *testing.T) {
	if err := Validate(mustParse(t, compliant)); err != nil {
		t.Fatalf("Validate compliant config: %v", err)
	}
}

func TestValidateViolations(t *testing.T) {
	tests := []struct {
		name string
		cfg  string
	}{
		{
			name: "no TLS",
			cfg: `server { listen 80; client_max_body_size 10m;
				location / { limit_req zone=z; proxy_pass http://10.0.0.2:80; } }`,
		},
		{
			name: "no certificate",
			cfg: `server { listen 443 ssl; client_max_body_size 10m;
				location / { limit_req zone=z; proxy_pass http://10.0.0.2:80; } }`,
		},
		{
			name: "no rate limit",
			cfg: `server { listen 443 ssl; ssl_certificate /x; client_max_body_size 10m;
				location / { proxy_pass http://10.0.0.2:80; } }`,
		},
		{
			name: "no body limit",
			cfg: `server { listen 443 ssl; ssl_certificate /x;
				location / { limit_req zone=z; proxy_pass http://10.0.0.2:80; } }`,
		},
		{
			name: "upstream to loopback",
			cfg: `limit_req_zone x zone=z:10m rate=1r/s;
				server { listen 443 ssl; ssl_certificate /x; client_max_body_size 10m;
				location / { limit_req zone=z; proxy_pass http://127.0.0.1:8080; } }`,
		},
		{
			name: "upstream to localhost",
			cfg: `limit_req_zone x zone=z:10m rate=1r/s;
				server { listen 443 ssl; ssl_certificate /x; client_max_body_size 10m;
				location / { limit_req zone=z; proxy_pass http://localhost:8080; } }`,
		},
		{
			name: "obsolete TLS protocol",
			cfg: `limit_req_zone x zone=z:10m rate=1r/s;
				server { listen 443 ssl; ssl_certificate /x; ssl_protocols TLSv1.1 TLSv1.2;
				client_max_body_size 10m;
				location / { limit_req zone=z; proxy_pass http://10.0.0.2:80; } }`,
		},
		{
			name: "server_tokens on",
			cfg: `limit_req_zone x zone=z:10m rate=1r/s;
				server { listen 443 ssl; ssl_certificate /x; server_tokens on;
				client_max_body_size 10m;
				location / { limit_req zone=z; proxy_pass http://10.0.0.2:80; } }`,
		},
		{
			name: "upstream with empty host",
			cfg: `limit_req_zone x zone=z:10m rate=1r/s;
				server { listen 443 ssl; ssl_certificate /x; client_max_body_size 10m;
				location / { limit_req zone=z; proxy_pass http://; } }`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(mustParse(t, tt.cfg))
			if !apperrors.Is(err, apperrors.ErrSecurityViolation) {
				t.Fatalf("err = %v, want ErrSecurityViolation", err)
			}
		})
	}
}

func TestUpstreamHost(t *testing.T) {
	tests := []struct {
		target string
		want   string
	}{
		{"http://10.0.0.2:8080", "10.0.0.2"},
		{"http://10.0.0.2:8080/path", "10.0.0.2"},
		{"10.0.0.2:8080", "10.0.0.2"},
		{"https://[::1]:443", "::1"},
		{"backend", "backend"},
	}
	for _, tt := range tests {
		if got := upstreamHost(tt.target); got != tt.want {
			t.Errorf("upstreamHost(%q) = %q, want %q", tt.target, got, tt.want)
		}
	}
}
