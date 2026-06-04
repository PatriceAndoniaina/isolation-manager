// Package logs récupère les journaux d'un conteneur depuis journald.
//
// Le conteneur tourne dans une unité systemd transitoire (voir pkg/nspawn) :
// la console du conteneur démarré avec --boot est capturée par le journal de
// cette unité. On lit donc via `journalctl -u <config.UnitName>.service`, ce qui
// présente deux avantages : les logs restent disponibles même après l'arrêt du
// conteneur, et l'unité ciblée est cohérente avec pkg/cgroups.
//
// journalctl passe par l'interface Journal, mockable dans les tests comme le
// Runner de pkg/nspawn.
package logs

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	"github.com/PatriceAndoniaina/isolation-manager/src/internal/log"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// unitSuffix : systemd-run produit une unité de type service.
const unitSuffix = ".service"

// Journal abstrait l'exécution de journalctl. Capture récupère un instantané
// borné des journaux ; Stream diffuse en continu (mode suivi) vers un writer
// jusqu'à l'annulation du contexte. Les tests injectent un faux Journal.
type Journal interface {
	Capture(ctx context.Context, argv []string) ([]byte, error)
	Stream(ctx context.Context, argv []string, out io.Writer) error
}

// systemJournal est l'implémentation réelle au-dessus de os/exec.
type systemJournal struct{}

// Capture exécute argv et renvoie sa sortie combinée. Le ctx porte le timeout.
func (systemJournal) Capture(ctx context.Context, argv []string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
}

// Stream exécute argv en diffusant la sortie vers out jusqu'à l'arrêt du ctx.
func (systemJournal) Stream(ctx context.Context, argv []string, out io.Writer) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

// Manager lit les journaux des conteneurs.
type Manager struct {
	journal Journal
}

// Option configure un Manager.
type Option func(*Manager)

// WithJournal injecte un Journal personnalisé (utilisé par les tests).
func WithJournal(j Journal) Option { return func(m *Manager) { m.journal = j } }

// New construit un Manager avec les valeurs par défaut, surchargeables.
func New(opts ...Option) *Manager {
	m := &Manager{journal: systemJournal{}}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Options paramètre la récupération des journaux.
type Options struct {
	Name   string // nom du conteneur
	Lines  int    // nombre de lignes (-n) ; <= 0 → omis
	Since  string // borne temporelle (--since) ; vide → omise
	Follow bool   // suivi en continu (-f)
}

// Fetch écrit les journaux du conteneur dans w. En mode suivi (Follow), la
// diffusion continue jusqu'à l'annulation du contexte (ex: Ctrl-C) ; sinon un
// instantané borné par config.DefaultExecTimeout est récupéré.
func (m *Manager) Fetch(ctx context.Context, w io.Writer, opts Options) error {
	if err := container.ValidateName(opts.Name); err != nil {
		return err
	}
	if opts.Lines < 0 {
		return apperrors.Wrap("logs", opts.Name, fmt.Errorf("lines must be >= 0, got %d", opts.Lines))
	}

	unit := config.UnitName(opts.Name) + unitSuffix
	argv := buildArgs(unit, opts)
	log.Audit("logs", opts.Name, log.Fields{"follow": opts.Follow, "lines": opts.Lines})

	if opts.Follow {
		return apperrors.Wrap("logs", opts.Name, m.journal.Stream(ctx, argv, w))
	}

	cctx, cancel := context.WithTimeout(ctx, config.DefaultExecTimeout)
	defer cancel()
	out, err := m.journal.Capture(cctx, argv)
	if err != nil {
		return apperrors.Wrap("logs", opts.Name,
			fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))))
	}
	_, err = w.Write(out)
	return err
}

// buildArgs construit la ligne de commande journalctl.
func buildArgs(unit string, opts Options) []string {
	argv := []string{"journalctl", "-u", unit, "--no-pager"}
	if opts.Lines > 0 {
		argv = append(argv, "-n", strconv.Itoa(opts.Lines))
	}
	if opts.Since != "" {
		argv = append(argv, "--since", opts.Since)
	}
	if opts.Follow {
		argv = append(argv, "-f")
	}
	return argv
}
