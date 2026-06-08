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

// reloadArgs choisit la commande de rechargement selon l'OS et le gestionnaire
// de services disponible : systemd (`systemctl reload nginx`) ou SysV
// (`service nginx reload`) sous Linux, sinon le signal natif `nginx -s reload`
// (autres OS, ou Linux sans gestionnaire de services détecté).
func (t *Tester) reloadArgs() []string {
	if t.goos == "linux" {
		switch {
		case t.look("systemctl"):
			return []string{"systemctl", "reload", "nginx"}
		case t.look("service"):
			return []string{"service", "nginx", "reload"}
		}
	}
	return []string{"nginx", "-s", "reload"}
}

// Reload recharge la configuration nginx à chaud, sans interruption de service,
// via la commande adaptée à l'OS hôte (voir reloadArgs). À n'exécuter qu'après
// un Check réussi pour ne pas propager une configuration invalide.
func (t *Tester) Reload(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, config.DefaultExecTimeout)
	defer cancel()

	argv := t.reloadArgs()
	out, err := t.runner.Run(ctx, argv)
	if err != nil {
		return out, fmt.Errorf("%s a échoué: %w", strings.Join(argv, " "), err)
	}
	return out, nil
}
