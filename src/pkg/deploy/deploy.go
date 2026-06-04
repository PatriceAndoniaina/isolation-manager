// Package deploy provisionne l'outil sur un serveur Linux distant et y exécute
// des commandes à distance.
//
// Flux de déploiement : connexion SSH → détection de l'OS/architecture
// (uname) → compilation croisée locale (GOOS=linux) → transfert via rsync →
// installation des dépendances (systemd-container) si demandé. Toutes les
// commandes externes (ssh, rsync, go) passent par l'interface Commander,
// mockable dans les tests comme le Runner de pkg/nspawn.
package deploy

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// defaultBuildPkg est le package compilé pour produire le binaire.
const defaultBuildPkg = "./src/cmd"

// remoteTmpPath est l'emplacement temporaire du binaire transféré, avant
// installation à son emplacement final (qui nécessite sudo).
const remoteTmpPath = "/tmp/isolation-manager.deploy"

// Cmd décrit une commande externe à exécuter (locale ou via ssh).
type Cmd struct {
	Name string
	Args []string
	Dir  string   // répertoire de travail (compilation)
	Env  []string // variables ajoutées à l'environnement (GOOS, GOARCH…)
}

// Streams regroupe les flux standard d'une commande interactive.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Commander abstrait l'exécution des commandes externes. Capture récupère la
// sortie combinée ; Interactive attache les flux standard (session distante).
type Commander interface {
	Capture(ctx context.Context, c Cmd) ([]byte, error)
	Interactive(ctx context.Context, c Cmd, s Streams) error
}

// systemCommander est l'implémentation réelle au-dessus de os/exec.
type systemCommander struct{}

func (systemCommander) Capture(ctx context.Context, c Cmd) ([]byte, error) {
	if c.Name == "" {
		return nil, fmt.Errorf("empty command")
	}
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	return cmd.CombinedOutput()
}

func (systemCommander) Interactive(ctx context.Context, c Cmd, s Streams) error {
	if c.Name == "" {
		return fmt.Errorf("empty command")
	}
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = append(os.Environ(), c.Env...)
	cmd.Stdin = s.In
	cmd.Stdout = s.Out
	cmd.Stderr = s.Err
	return cmd.Run()
}

// Target identifie le serveur distant et les paramètres de connexion SSH.
type Target struct {
	Host string
	User string
	Port int    // 0 → port SSH par défaut
	Key  string // chemin de la clé privée, optionnel
}

// dest renvoie la cible "user@host".
func (t Target) dest() string { return t.User + "@" + t.Host }

// Options paramètre un déploiement.
type Options struct {
	Target
	RemotePath  string // emplacement d'installation distant
	Source      string // racine du module à compiler (go build)
	InstallDeps bool   // installer systemd-container si absent
}

func (o Options) validate() error {
	switch {
	case o.Host == "":
		return apperrors.Wrap("deploy", "", fmt.Errorf("host requis"))
	case o.User == "":
		return apperrors.Wrap("deploy", o.Host, fmt.Errorf("user requis"))
	case o.RemotePath == "":
		return apperrors.Wrap("deploy", o.Host, fmt.Errorf("remote-path requis"))
	}
	return nil
}

// Deployer orchestre le déploiement et l'exécution distante.
type Deployer struct {
	cmd      Commander
	buildPkg string
}

// Option configure un Deployer.
type Option func(*Deployer)

// WithCommander injecte un Commander personnalisé (utilisé par les tests).
func WithCommander(c Commander) Option { return func(d *Deployer) { d.cmd = c } }

// New construit un Deployer avec les valeurs par défaut, surchargeables.
func New(opts ...Option) *Deployer {
	d := &Deployer{cmd: systemCommander{}, buildPkg: defaultBuildPkg}
	for _, o := range opts {
		o(d)
	}
	return d
}

// Deploy provisionne le serveur : détection, compilation, transfert, dépendances.
func (d *Deployer) Deploy(ctx context.Context, w io.Writer, opts Options) error {
	if err := opts.validate(); err != nil {
		return err
	}

	fmt.Fprintf(w, "→ détection de l'OS sur %s…\n", opts.Host)
	goarch, err := d.detect(ctx, opts.Target)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  Linux détecté, architecture %s\n", goarch)

	fmt.Fprintf(w, "→ compilation croisée (linux/%s)…\n", goarch)
	bin, err := d.build(ctx, opts.Source, goarch)
	if err != nil {
		return apperrors.Wrap("deploy", opts.Host, err)
	}
	defer func() { _ = os.Remove(bin) }()

	fmt.Fprintf(w, "→ transfert via rsync vers %s:%s…\n", opts.Host, opts.RemotePath)
	if err := d.transfer(ctx, opts.Target, bin, opts.RemotePath); err != nil {
		return err
	}

	if opts.InstallDeps {
		fmt.Fprintln(w, "→ vérification / installation de systemd-container…")
		if err := d.ensureDeps(ctx, opts.Target); err != nil {
			return err
		}
	} else if err := d.verifyDeps(ctx, opts.Target); err != nil {
		fmt.Fprintf(w, "  ⚠️  %v\n", err)
	}

	fmt.Fprintf(w, "✅ déployé : %s sur %s\n", opts.RemotePath, opts.Host)
	return nil
}

// Exec exécute une commande de l'outil sur le serveur distant via SSH, avec un
// pseudo-terminal pour les sous-commandes interactives (ssh, stats --watch).
func (d *Deployer) Exec(ctx context.Context, t Target, remotePath string, args []string, s Streams) error {
	if t.Host == "" {
		return apperrors.Wrap("remote", "", fmt.Errorf("host requis"))
	}
	remote := "sudo " + remotePath
	if len(args) > 0 {
		remote += " " + strings.Join(shellQuote(args), " ")
	}
	argv := append([]string{"-t"}, sshOpts(t)...)
	argv = append(argv, t.dest(), remote)
	return apperrors.Wrap("remote", t.Host, d.cmd.Interactive(ctx, Cmd{Name: "ssh", Args: argv}, s))
}

// detect interroge uname sur le serveur et renvoie l'architecture Go.
func (d *Deployer) detect(ctx context.Context, t Target) (string, error) {
	out, err := d.cmd.Capture(ctx, sshCmd(t, "uname -s -m"))
	if err != nil {
		return "", apperrors.Wrap("deploy", t.Host,
			fmt.Errorf("ssh uname: %w: %s", err, strings.TrimSpace(string(out))))
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return "", apperrors.Wrap("deploy", t.Host, fmt.Errorf("réponse uname inattendue: %q", string(out)))
	}
	if fields[0] != "Linux" {
		return "", apperrors.Wrap("deploy", t.Host, fmt.Errorf("OS non supporté: %s (Linux requis)", fields[0]))
	}
	goarch, err := mapArch(fields[1])
	if err != nil {
		return "", apperrors.Wrap("deploy", t.Host, err)
	}
	return goarch, nil
}

// build compile le binaire pour linux/goarch et renvoie son chemin local.
func (d *Deployer) build(ctx context.Context, source, goarch string) (string, error) {
	if source == "" {
		source = "."
	}
	out := filepath.Join(os.TempDir(), "isolation-manager-linux-"+goarch)
	c := Cmd{
		Name: "go",
		Args: []string{"build", "-o", out, d.buildPkg},
		Dir:  source,
		Env:  []string{"GOOS=linux", "GOARCH=" + goarch, "CGO_ENABLED=0"},
	}
	if o, err := d.cmd.Capture(ctx, c); err != nil {
		return "", fmt.Errorf("go build: %w: %s", err, strings.TrimSpace(string(o)))
	}
	return out, nil
}

// transfer copie le binaire via rsync puis l'installe à son emplacement final.
func (d *Deployer) transfer(ctx context.Context, t Target, localBin, remotePath string) error {
	transport := "ssh " + strings.Join(sshOpts(t), " ")
	rsync := Cmd{Name: "rsync", Args: []string{"-az", "-e", transport, localBin, t.dest() + ":" + remoteTmpPath}}
	if o, err := d.cmd.Capture(ctx, rsync); err != nil {
		return apperrors.Wrap("deploy", t.Host, fmt.Errorf("rsync: %w: %s", err, strings.TrimSpace(string(o))))
	}

	install := fmt.Sprintf("sudo install -m 0755 %s %s && rm -f %s", remoteTmpPath, remotePath, remoteTmpPath)
	if o, err := d.cmd.Capture(ctx, sshCmd(t, install)); err != nil {
		return apperrors.Wrap("deploy", t.Host, fmt.Errorf("install: %w: %s", err, strings.TrimSpace(string(o))))
	}
	return nil
}

// ensureDeps installe systemd-container s'il est absent (apt/dnf/pacman).
func (d *Deployer) ensureDeps(ctx context.Context, t Target) error {
	if o, err := d.cmd.Capture(ctx, sshCmd(t, installScript)); err != nil {
		return apperrors.Wrap("deploy", t.Host,
			fmt.Errorf("install dépendances: %w: %s", err, strings.TrimSpace(string(o))))
	}
	return nil
}

// verifyDeps signale l'absence de systemd-nspawn sans rien installer.
func (d *Deployer) verifyDeps(ctx context.Context, t Target) error {
	check := "command -v systemd-nspawn >/dev/null 2>&1"
	if _, err := d.cmd.Capture(ctx, sshCmd(t, check)); err != nil {
		return fmt.Errorf("systemd-nspawn absent sur %s (paquet systemd-container)", t.Host)
	}
	return nil
}

// installScript vérifie puis installe systemd-container selon la distro.
const installScript = `set -e
if command -v systemd-nspawn >/dev/null 2>&1; then echo "systemd-nspawn déjà présent"; exit 0; fi
if command -v apt-get >/dev/null 2>&1; then sudo apt-get update -qq && sudo apt-get install -y systemd-container
elif command -v dnf >/dev/null 2>&1; then sudo dnf install -y systemd-container
elif command -v pacman >/dev/null 2>&1; then sudo pacman -Sy --noconfirm systemd
else echo "gestionnaire de paquets non supporté" >&2; exit 1
fi`

// sshCmd construit une commande ssh exécutant remote sur la cible.
func sshCmd(t Target, remote string) Cmd {
	argv := append(sshOpts(t), t.dest(), remote)
	return Cmd{Name: "ssh", Args: argv}
}

// sshOpts renvoie les options ssh communes (port, clé, TOFU, timeout).
func sshOpts(t Target) []string {
	opts := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
	}
	if t.Port != 0 {
		opts = append(opts, "-p", strconv.Itoa(t.Port))
	}
	if t.Key != "" {
		opts = append(opts, "-i", t.Key)
	}
	return opts
}

// mapArch traduit la sortie uname -m en architecture Go (GOARCH).
func mapArch(uname string) (string, error) {
	switch uname {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("architecture non supportée: %s", uname)
	}
}

// shellQuote met chaque argument entre guillemets simples (échappement sûr)
// pour le passage à la commande distante.
func shellQuote(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
	}
	return out
}
