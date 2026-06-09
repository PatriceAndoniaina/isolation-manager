package nginx

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
)

// Runner abstrait l'exécution de la commande nginx, mockable dans les tests
// (comme le Runner de pkg/nspawn). Chaque appel est borné par le ctx.
type Runner interface {
	Run(ctx context.Context, argv []string) ([]byte, error)
}

// execRunner exécute réellement nginx via le système.
type execRunner struct{}

func (execRunner) Run(ctx context.Context, argv []string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
}

// Tester délègue la validation de configuration au binaire nginx (`nginx -t`),
// qui vérifie la syntaxe et la sémantique réelles (directives, includes…) —
// complémentaire de Validate, qui applique nos règles de sécurité hors-ligne.
type Tester struct {
	runner Runner
	look   func(string) bool // présence d'un binaire (systemctl/service)
	goos   string            // OS hôte (runtime.GOOS), surchargeable en test
}

// TesterOption configure un Tester.
type TesterOption func(*Tester)

// WithRunner injecte un Runner personnalisé (utilisé par les tests).
func WithRunner(r Runner) TesterOption { return func(t *Tester) { t.runner = r } }

// WithLooker surcharge la détection de présence d'un binaire (utilisé par les tests).
func WithLooker(f func(string) bool) TesterOption { return func(t *Tester) { t.look = f } }

// WithGOOS surcharge l'OS détecté (utilisé par les tests).
func WithGOOS(goos string) TesterOption { return func(t *Tester) { t.goos = goos } }

// NewTester construit un Tester (Runner réel par défaut, surchargeable).
func NewTester(opts ...TesterOption) *Tester {
	t := &Tester{runner: execRunner{}, look: lookPath, goos: runtime.GOOS}
	for _, o := range opts {
		o(t)
	}
	return t
}

// lookPath indique si un binaire est présent dans le PATH.
func lookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Check exécute `nginx -t` (et `-c <file>` si file est non vide), sous un
// timeout. Renvoie la sortie combinée de nginx ; une configuration invalide
// (code de sortie non nul) produit une erreur.
func (t *Tester) Check(ctx context.Context, file string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, config.DefaultExecTimeout)
	defer cancel()

	argv := []string{"nginx", "-t"}
	if file != "" {
		argv = append(argv, "-c", file)
	}
	out, err := t.runner.Run(ctx, argv)
	if err != nil {
		return out, fmt.Errorf("nginx -t a échoué: %w", err)
	}
	return out, nil
}

// run exécute argv sous un timeout et enveloppe l'erreur avec la commande.
func (t *Tester) run(ctx context.Context, argv []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, config.DefaultExecTimeout)
	defer cancel()
	out, err := t.runner.Run(ctx, argv)
	if err != nil {
		return out, fmt.Errorf("%s a échoué: %w", strings.Join(argv, " "), err)
	}
	return out, nil
}

// serviceCmd renvoie la commande du gestionnaire de services pour action
// ("reload"/"restart") sous Linux : systemd (`systemctl <action> nginx`) puis
// SysV (`service nginx <action>`). Renvoie nil si aucun n'est détecté (autres
// OS, ou Linux sans gestionnaire) — l'appelant retombe sur les signaux nginx.
func (t *Tester) serviceCmd(action string) []string {
	if t.goos == "linux" {
		switch {
		case t.look("systemctl"):
			return []string{"systemctl", action, "nginx"}
		case t.look("service"):
			return []string{"service", "nginx", action}
		}
	}
	return nil
}

// Reload recharge la configuration nginx à chaud, sans interruption de service,
// via la commande adaptée à l'OS hôte. À n'exécuter qu'après un Check réussi
// pour ne pas propager une configuration invalide.
func (t *Tester) Reload(ctx context.Context) ([]byte, error) {
	if argv := t.serviceCmd("reload"); argv != nil {
		return t.run(ctx, argv)
	}
	return t.run(ctx, []string{"nginx", "-s", "reload"})
}

// Restart redémarre nginx (arrêt complet puis démarrage), via le gestionnaire
// de services s'il est disponible ; sinon `nginx -s stop` (best-effort, ignoré
// si nginx n'est pas lancé) suivi de `nginx`.
func (t *Tester) Restart(ctx context.Context) ([]byte, error) {
	if argv := t.serviceCmd("restart"); argv != nil {
		return t.run(ctx, argv)
	}
	_, _ = t.run(ctx, []string{"nginx", "-s", "stop"})
	return t.run(ctx, []string{"nginx"})
}

// Status renvoie l'état du service nginx via le gestionnaire de services
// (`systemctl status nginx` ou `service nginx status`). L'erreur du runner est
// renvoyée brute (un code non nul signale simplement « inactif », pas un échec
// de commande). Sur un hôte sans gestionnaire de services, renvoie une erreur
// explicite plutôt qu'un statut deviné.
func (t *Tester) Status(ctx context.Context) ([]byte, error) {
	argv := t.serviceCmd("status")
	if argv == nil {
		return nil, fmt.Errorf("statut indisponible : aucun gestionnaire de services (systemctl/service) sur cet hôte")
	}
	ctx, cancel := context.WithTimeout(ctx, config.DefaultExecTimeout)
	defer cancel()
	return t.runner.Run(ctx, argv)
}
