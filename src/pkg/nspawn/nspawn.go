// Package nspawn implémente l'interface container.Containerizer au-dessus de
// systemd-nspawn / machinectl.
//
// Toute exécution de commande passe par l'interface Runner, ce qui permet de
// mocker le système externe dans les tests (règle : "Mocks pour les systèmes
// externes"). Chaque appel système est borné par un timeout et journalisé
// (audit trail), et un garde-fou rejette les drapeaux interdits (--privileged,
// --capability) avant toute exécution (security gate).
package nspawn

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	"github.com/PatriceAndoniaina/isolation-manager/src/internal/log"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// forbiddenFlags liste les drapeaux interdits par la politique de sécurité.
// Le garde-fou les rejette quel que soit le Runner utilisé.
var forbiddenFlags = []string{"--privileged", "--capability"}

// Runner abstrait l'exécution d'une commande externe (systemd-nspawn,
// machinectl). L'implémentation par défaut s'appuie sur exec.CommandContext ;
// les tests injectent un faux Runner.
type Runner interface {
	Run(ctx context.Context, argv []string) ([]byte, error)
}

// execRunner exécute réellement les commandes via le système.
type execRunner struct{}

// Run lance argv[0] avec ses arguments et retourne la sortie combinée.
// Le ctx porte le timeout : son expiration tue le processus.
func (execRunner) Run(ctx context.Context, argv []string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	return cmd.CombinedOutput()
}

// Manager est l'implémentation systemd-nspawn de container.Containerizer.
type Manager struct {
	runner      Runner
	store       *fileStore
	machinesDir string // emplacement des rootfs (par défaut config.MachinesDir)
}

// Vérification à la compilation que Manager respecte bien le contrat.
var _ container.Containerizer = (*Manager)(nil)

// Option configure un Manager.
type Option func(*Manager)

// WithRunner injecte un Runner personnalisé (utilisé par les tests).
func WithRunner(r Runner) Option { return func(m *Manager) { m.runner = r } }

// WithMachinesDir surcharge l'emplacement des rootfs.
func WithMachinesDir(dir string) Option { return func(m *Manager) { m.machinesDir = dir } }

// WithStateDir surcharge le répertoire de persistance des métadonnées.
func WithStateDir(dir string) Option { return func(m *Manager) { m.store = newFileStore(dir) } }

// New construit un Manager avec les valeurs par défaut, surchargeables via Option.
func New(opts ...Option) *Manager {
	m := &Manager{
		runner:      execRunner{},
		store:       newFileStore(config.StateDir),
		machinesDir: config.MachinesDir,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// guardArgs rejette les commandes contenant un drapeau interdit. Exécuté avant
// chaque appel système, indépendamment du Runner (défense en profondeur).
func guardArgs(argv []string) error {
	for _, a := range argv {
		for _, bad := range forbiddenFlags {
			if a == bad || strings.HasPrefix(a, bad+"=") {
				return fmt.Errorf("%w: forbidden flag %q", apperrors.ErrSecurityViolation, a)
			}
		}
	}
	return nil
}

// exec applique le garde-fou, journalise l'opération (audit), puis exécute la
// commande via le Runner. Un dépassement de deadline est traduit en ErrTimeout.
func (m *Manager) exec(ctx context.Context, op, name string, argv []string) ([]byte, error) {
	if err := guardArgs(argv); err != nil {
		return nil, apperrors.Wrap(op, name, err)
	}
	log.Audit(op, name, log.Fields{"cmd": argv[0], "args": argv[1:]})

	out, err := m.runner.Run(ctx, argv)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("%w: %v", apperrors.ErrTimeout, err)
		}
		return out, apperrors.Wrap(op, name, err)
	}
	return out, nil
}

// withTimeout garantit qu'aucune commande ne s'exécute sans borne temporelle.
func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}
