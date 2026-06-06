// Package preflight vérifie que les binaires externes requis par une opération
// sont présents et, le cas échéant, les installe automatiquement via le
// gestionnaire de paquets de la distribution (apt/dnf/pacman).
//
// L'objectif : avant de lancer une commande (start, ssh, logs…), s'assurer que
// systemd-nspawn, machinectl, ssh, etc. existent sur l'hôte ; sinon les
// installer. Les accès système (présence d'un binaire, installation) passent
// par l'interface Commander, mockable dans les tests comme pkg/nspawn.
package preflight

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// pkgTable associe un binaire au paquet qui le fournit, par gestionnaire.
var pkgTable = map[string]map[string]string{
	"systemd-nspawn": {"apt": "systemd-container", "dnf": "systemd-container", "pacman": "systemd"},
	"machinectl":     {"apt": "systemd-container", "dnf": "systemd-container", "pacman": "systemd"},
	"systemd-run":    {"apt": "systemd", "dnf": "systemd", "pacman": "systemd"},
	"journalctl":     {"apt": "systemd", "dnf": "systemd", "pacman": "systemd"},
	"ssh":            {"apt": "openssh-client", "dnf": "openssh-clients", "pacman": "openssh"},
	"ssh-keygen":     {"apt": "openssh-client", "dnf": "openssh-clients", "pacman": "openssh"},
	"rsync":          {"apt": "rsync", "dnf": "rsync", "pacman": "rsync"},
	"nginx":          {"apt": "nginx", "dnf": "nginx", "pacman": "nginx"},
}

// Commander abstrait l'accès système : Look teste la présence d'un binaire,
// Run exécute une commande (détection du gestionnaire, installation).
type Commander interface {
	Look(name string) bool
	Run(ctx context.Context, argv []string) ([]byte, error)
}

// systemCommander est l'implémentation réelle au-dessus de os/exec.
type systemCommander struct{}

func (systemCommander) Look(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func (systemCommander) Run(ctx context.Context, argv []string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
}

// Checker vérifie et installe les dépendances.
type Checker struct {
	cmd Commander
}

// Option configure un Checker.
type Option func(*Checker)

// WithCommander injecte un Commander personnalisé (utilisé par les tests).
func WithCommander(c Commander) Option { return func(k *Checker) { k.cmd = c } }

// New construit un Checker avec les valeurs par défaut, surchargeables.
func New(opts ...Option) *Checker {
	k := &Checker{cmd: systemCommander{}}
	for _, o := range opts {
		o(k)
	}
	return k
}

// Ensure vérifie que toutes les commandes existent. Les manquantes sont
// installées si autoInstall est vrai ; sinon (ou si l'installation est
// impossible) une ErrDependencyMissing est renvoyée. La progression est écrite
// sur w.
func (k *Checker) Ensure(ctx context.Context, w io.Writer, commands []string, autoInstall bool) error {
	missing := k.missing(commands)
	if len(missing) == 0 {
		return nil
	}

	if !autoInstall {
		return fmt.Errorf("%w: %s (installez-les ou retirez --auto-install=false)",
			apperrors.ErrDependencyMissing, strings.Join(missing, ", "))
	}

	mgr := k.detectManager()
	if mgr == "" {
		return fmt.Errorf("%w: %s ; aucun gestionnaire de paquets (apt/dnf/pacman) détecté",
			apperrors.ErrDependencyMissing, strings.Join(missing, ", "))
	}

	pkgs := packagesFor(missing, mgr)
	fmt.Fprintf(w, "→ dépendances manquantes (%s) : %s — installation de %s…\n",
		mgr, strings.Join(missing, ", "), strings.Join(pkgs, " "))
	if out, err := k.cmd.Run(ctx, installCmd(mgr, pkgs)); err != nil {
		return fmt.Errorf("%w: installation échouée: %v: %s",
			apperrors.ErrDependencyMissing, err, strings.TrimSpace(string(out)))
	}

	if still := k.missing(missing); len(still) > 0 {
		return fmt.Errorf("%w: toujours absent après installation: %s",
			apperrors.ErrDependencyMissing, strings.Join(still, ", "))
	}
	return nil
}

// missing renvoie les commandes absentes parmi celles fournies.
func (k *Checker) missing(commands []string) []string {
	var out []string
	for _, c := range commands {
		if !k.cmd.Look(c) {
			out = append(out, c)
		}
	}
	return out
}

// detectManager renvoie le gestionnaire de paquets disponible, ou "".
// L'ordre de préférence est fixe (apt, dnf, pacman) pour un comportement
// déterministe sur les rares hôtes en ayant plusieurs.
func (k *Checker) detectManager() string {
	for _, m := range []struct{ bin, mgr string }{
		{"apt-get", "apt"}, {"dnf", "dnf"}, {"pacman", "pacman"},
	} {
		if k.cmd.Look(m.bin) {
			return m.mgr
		}
	}
	return ""
}

// packagesFor traduit les binaires manquants en paquets (dédupliqués).
func packagesFor(cmds []string, mgr string) []string {
	seen := map[string]bool{}
	var pkgs []string
	for _, c := range cmds {
		p := pkgFor(c, mgr)
		if !seen[p] {
			seen[p] = true
			pkgs = append(pkgs, p)
		}
	}
	return pkgs
}

// pkgFor renvoie le paquet fournissant cmd pour le gestionnaire mgr ; à défaut
// de correspondance, le nom de la commande est utilisé comme nom de paquet.
func pkgFor(cmd, mgr string) string {
	if m, ok := pkgTable[cmd]; ok {
		if p, ok := m[mgr]; ok {
			return p
		}
	}
	return cmd
}

// installCmd construit la commande d'installation pour le gestionnaire mgr.
func installCmd(mgr string, pkgs []string) []string {
	switch mgr {
	case "apt":
		return append([]string{"sudo", "apt-get", "install", "-y"}, pkgs...)
	case "dnf":
		return append([]string{"sudo", "dnf", "install", "-y"}, pkgs...)
	case "pacman":
		return append([]string{"sudo", "pacman", "-S", "--noconfirm"}, pkgs...)
	}
	return nil
}
