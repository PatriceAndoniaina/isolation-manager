package nginx

import (
	"fmt"
	"net"
	"strings"

	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// forbiddenUpstreamHosts liste les hôtes interdits comme upstream : la loopback
// et l'adresse joker désignent l'hôte lui-même (rules/security.md → "Aucun
// upstream vers l'hôte").
var forbiddenUpstreamHosts = map[string]bool{
	"localhost": true,
	"127.0.0.1": true,
	"::1":       true,
	"0.0.0.0":   true,
}

// obsoleteTLS liste les versions de protocole TLS/SSL jugées obsolètes : leur
// présence dans ssl_protocols est une violation de sécurité.
var obsoleteTLS = map[string]bool{
	"SSLv2":   true,
	"SSLv3":   true,
	"TLSv1":   true,
	"TLSv1.1": true,
}

// Validate vérifie qu'une configuration respecte les règles de sécurité du
// proxy (rules/security.md → Nginx Validation) : TLS imposé, rate limiting,
// limite de taille de requête, et aucun upstream vers l'hôte. Tout manquement
// retourne une ErrSecurityViolation.
func Validate(dirs []*Directive) error {
	var hasTLS, hasCert, hasRateLimit, hasBodyLimit bool
	var secErr error
	// fail enregistre la première violation rencontrée pendant le parcours.
	fail := func(err error) {
		if secErr == nil {
			secErr = err
		}
	}

	Walk(dirs, func(d *Directive) {
		switch d.Name {
		case "listen":
			for _, a := range d.Args {
				if a == "ssl" {
					hasTLS = true
				}
			}
		case "ssl_certificate":
			hasCert = true
		case "limit_req":
			hasRateLimit = true
		case "client_max_body_size":
			hasBodyLimit = true
		case "proxy_pass":
			if len(d.Args) > 0 {
				if err := validateUpstream(d.Args[0]); err != nil {
					fail(err)
				}
			}
		case "ssl_protocols":
			for _, a := range d.Args {
				if obsoleteTLS[a] {
					fail(fmt.Errorf("%w: protocole TLS obsolète %q (ligne %d)",
						apperrors.ErrSecurityViolation, a, d.Line))
				}
			}
		case "server_tokens":
			if len(d.Args) > 0 && d.Args[0] == "on" {
				fail(fmt.Errorf("%w: server_tokens on expose la version nginx (ligne %d)",
					apperrors.ErrSecurityViolation, d.Line))
			}
		}
	})

	if secErr != nil {
		return secErr
	}
	if !hasTLS || !hasCert {
		return fmt.Errorf("%w: TLS non imposé (listen ... ssl + ssl_certificate requis)",
			apperrors.ErrSecurityViolation)
	}
	if !hasRateLimit {
		return fmt.Errorf("%w: rate limiting absent (limit_req requis)",
			apperrors.ErrSecurityViolation)
	}
	if !hasBodyLimit {
		return fmt.Errorf("%w: limite de taille de requête absente (client_max_body_size requis)",
			apperrors.ErrSecurityViolation)
	}
	return nil
}

// validateUpstream rejette un proxy_pass pointant vers l'hôte ou la loopback.
func validateUpstream(target string) error {
	host := upstreamHost(target)
	if host == "" {
		return fmt.Errorf("%w: upstream invalide %q", apperrors.ErrSecurityViolation, target)
	}
	if forbiddenUpstreamHosts[strings.ToLower(host)] {
		return fmt.Errorf("%w: upstream vers l'hôte interdit (%s)", apperrors.ErrSecurityViolation, host)
	}
	return nil
}

// upstreamHost extrait l'hôte d'une cible proxy_pass (schéma et port optionnels,
// IPv6 entre crochets géré).
func upstreamHost(target string) string {
	s := target
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	if h, _, err := net.SplitHostPort(s); err == nil {
		return h
	}
	return strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
}
