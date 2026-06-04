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

// Validate vérifie qu'une configuration respecte les règles de sécurité du
// proxy (rules/security.md → Nginx Validation) : TLS imposé, rate limiting,
// limite de taille de requête, et aucun upstream vers l'hôte. Tout manquement
// retourne une ErrSecurityViolation.
func Validate(dirs []*Directive) error {
	var hasTLS, hasCert, hasRateLimit, hasBodyLimit bool
	var upstreamErr error

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
				if err := validateUpstream(d.Args[0]); err != nil && upstreamErr == nil {
					upstreamErr = err
				}
			}
		}
	})

	if upstreamErr != nil {
		return upstreamErr
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
