package nginx

import (
	"bytes"
	"fmt"
	"net"
	"regexp"
	"strings"
	"text/template"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// serverNameRE valide un server_name (domaine, sous-domaine ou joker).
var serverNameRE = regexp.MustCompile(`^[a-zA-Z0-9.*_-]{1,253}$`)

// SiteOptions paramètre la génération d'un server block de reverse proxy.
type SiteOptions struct {
	Name        string // nom du conteneur (commentaires + zone de rate limit)
	ServerName  string // nom de domaine servi (server_name)
	Upstream    string // adresse du conteneur host:port (jamais l'hôte)
	TLSCertPath string // chemin du certificat TLS
	TLSKeyPath  string // chemin de la clé privée TLS
}

// siteTemplate produit deux server blocks : redirection 80→443 et le proxy TLS
// durci (rate limiting, limite de taille, en-têtes de proxy).
const siteTemplate = `# Reverse proxy généré pour le conteneur {{.Name}}
# rules/security.md → Nginx Validation : TLS imposé, rate limit, pas d'upstream hôte.
limit_req_zone $binary_remote_addr zone={{.Zone}}:10m rate={{.Rate}};

server {
    listen 80;
    server_name {{.ServerName}};
    # Redirection systématique vers HTTPS (TLS imposé).
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl;
    server_name {{.ServerName}};

    ssl_certificate {{.TLSCertPath}};
    ssl_certificate_key {{.TLSKeyPath}};
    ssl_protocols TLSv1.2 TLSv1.3;

    # Limite de taille des requêtes.
    client_max_body_size {{.MaxBodySize}};

    location / {
        # Rate limiting sur le proxy.
        limit_req zone={{.Zone}} burst={{.Burst}} nodelay;

        proxy_pass http://{{.Upstream}};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`

// Generator génère des configurations de reverse proxy.
type Generator struct {
	tmpl *template.Template
}

// NewGenerator construit un Generator prêt à l'emploi.
func NewGenerator() *Generator {
	return &Generator{tmpl: template.Must(template.New("site").Parse(siteTemplate))}
}

// Render produit la configuration nginx du reverse proxy pour le conteneur.
// La sortie est relue (Parse) et validée (Validate) avant d'être renvoyée : une
// configuration non conforme aux règles de sécurité est une erreur, jamais un
// résultat exploitable (defense in depth).
func (g *Generator) Render(opts SiteOptions) (string, error) {
	if err := opts.validate(); err != nil {
		return "", err
	}

	data := struct {
		SiteOptions
		Zone        string
		Rate        string
		Burst       int
		MaxBodySize string
	}{
		SiteOptions: opts,
		Zone:        "rl_" + opts.Name,
		Rate:        config.NginxRateLimit,
		Burst:       config.NginxRateBurst,
		MaxBodySize: config.NginxMaxBodySize,
	}

	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, data); err != nil {
		return "", apperrors.Wrap("nginx", opts.Name, err)
	}
	out := buf.String()

	parsed, err := Parse(out)
	if err != nil {
		return "", apperrors.Wrap("nginx", opts.Name, err)
	}
	if err := Validate(parsed); err != nil {
		return "", apperrors.Wrap("nginx", opts.Name, err)
	}
	return out, nil
}

// validate contrôle les options avant génération.
func (o SiteOptions) validate() error {
	if err := container.ValidateName(o.Name); err != nil {
		return err
	}
	if !serverNameRE.MatchString(o.ServerName) {
		return apperrors.Wrap("nginx", o.Name, fmt.Errorf("server_name invalide: %q", o.ServerName))
	}
	if err := validatePath("tls-cert", o.TLSCertPath); err != nil {
		return apperrors.Wrap("nginx", o.Name, err)
	}
	if err := validatePath("tls-key", o.TLSKeyPath); err != nil {
		return apperrors.Wrap("nginx", o.Name, err)
	}
	// L'upstream doit être un host:port explicite…
	if _, _, err := net.SplitHostPort(o.Upstream); err != nil {
		return apperrors.Wrap("nginx", o.Name, fmt.Errorf("upstream doit être au format host:port: %w", err))
	}
	// …et ne jamais pointer vers l'hôte.
	if err := validateUpstream(o.Upstream); err != nil {
		return apperrors.Wrap("nginx", o.Name, err)
	}
	return nil
}

// validatePath rejette les chemins vides ou porteurs de caractères qui
// briseraient la syntaxe nginx (injection de directives).
func validatePath(field, p string) error {
	if p == "" {
		return fmt.Errorf("%s requis (TLS imposé)", field)
	}
	if strings.ContainsAny(p, " \t\r\n;{}#\"'") {
		return fmt.Errorf("%s invalide (caractères interdits): %q", field, p)
	}
	return nil
}
