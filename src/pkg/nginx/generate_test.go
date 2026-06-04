package nginx

import (
	"strings"
	"testing"

	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

func validOpts() SiteOptions {
	return SiteOptions{
		Name:        "user01",
		ServerName:  "user01.example.com",
		Upstream:    "10.0.0.2:8080",
		TLSCertPath: "/etc/ssl/user01.crt",
		TLSKeyPath:  "/etc/ssl/user01.key",
	}
}

func TestRenderProducesCompliantConfig(t *testing.T) {
	out, err := NewGenerator().Render(validOpts())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	// Render valide déjà sa sortie ; on confirme le contenu attendu.
	for _, want := range []string{
		"listen 443 ssl;",
		"ssl_certificate /etc/ssl/user01.crt;",
		"client_max_body_size 10m;",
		"limit_req zone=rl_user01 burst=20 nodelay;",
		"proxy_pass http://10.0.0.2:8080;",
		"return 301 https://$host$request_uri;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered config missing %q", want)
		}
	}

	// La sortie doit être reparsable et conforme.
	dirs, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parse rendered config: %v", err)
	}
	if err := Validate(dirs); err != nil {
		t.Fatalf("rendered config not compliant: %v", err)
	}
}

func TestRenderRejectsInvalid(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SiteOptions)
		want   error
	}{
		{"invalid container name", func(o *SiteOptions) { o.Name = "Bad/Name" }, apperrors.ErrInvalidName},
		{"upstream to host", func(o *SiteOptions) { o.Upstream = "127.0.0.1:8080" }, apperrors.ErrSecurityViolation},
		{"upstream without port", func(o *SiteOptions) { o.Upstream = "10.0.0.2" }, nil},
		{"empty server name", func(o *SiteOptions) { o.ServerName = "" }, nil},
		{"server name with space", func(o *SiteOptions) { o.ServerName = "bad name" }, nil},
		{"missing cert", func(o *SiteOptions) { o.TLSCertPath = "" }, nil},
		{"missing key", func(o *SiteOptions) { o.TLSKeyPath = "" }, nil},
		{"cert path injection", func(o *SiteOptions) { o.TLSCertPath = "/x; evil" }, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := validOpts()
			tt.mutate(&opts)
			_, err := NewGenerator().Render(opts)
			if err == nil {
				t.Fatalf("Render(%s) = nil, want error", tt.name)
			}
			if tt.want != nil && !apperrors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestRenderDeterministic(t *testing.T) {
	g := NewGenerator()
	a, err := g.Render(validOpts())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	b, err := g.Render(validOpts())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if a != b {
		t.Error("Render is not deterministic")
	}
}
