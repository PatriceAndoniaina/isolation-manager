package nginx

import (
	"context"
	"fmt"
	"os/exec"

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
}

// TesterOption configure un Tester.
type TesterOption func(*Tester)

// WithRunner injecte un Runner personnalisé (utilisé par les tests).
func WithRunner(r Runner) TesterOption { return func(t *Tester) { t.runner = r } }

// NewTester construit un Tester (Runner réel par défaut, surchargeable).
func NewTester(opts ...TesterOption) *Tester {
	t := &Tester{runner: execRunner{}}
	for _, o := range opts {
		o(t)
	}
	return t
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
