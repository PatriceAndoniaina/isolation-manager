// Package ssh gère les paires de clés des conteneurs et l'ouverture de sessions
// SSH interactives durcies.
//
// Conformément à rules/security.md → "SSH Hardening" : authentification par clé
// uniquement (jamais par mot de passe), clés ED25519, forwarding d'agent coupé,
// port applicatif > 10000 redirigé sur la loopback hôte, et clés privées en
// permissions strictes (0600). Les commandes externes (ssh-keygen, ssh) passent
// par l'interface Executor, mockable dans les tests comme le Runner de pkg/nspawn.
package ssh

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	"github.com/PatriceAndoniaina/isolation-manager/src/internal/log"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// Streams regroupe les flux standard attachés à une session interactive.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Executor abstrait l'exécution des commandes externes. Capture récupère la
// sortie d'un outil non interactif (ssh-keygen) ; Interactive attache les flux
// standard pour une session vivante (ssh). Les tests injectent un faux Executor.
type Executor interface {
	Capture(ctx context.Context, argv []string) ([]byte, error)
	Interactive(ctx context.Context, argv []string, s Streams) error
}

// systemExecutor est l'implémentation réelle au-dessus de os/exec.
type systemExecutor struct{}

// Capture exécute argv et renvoie sa sortie combinée. Le ctx porte le timeout.
func (systemExecutor) Capture(ctx context.Context, argv []string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
}

// Interactive exécute argv en branchant les flux standard fournis.
func (systemExecutor) Interactive(ctx context.Context, argv []string, s Streams) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty command")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = s.In
	cmd.Stdout = s.Out
	cmd.Stderr = s.Err
	return cmd.Run()
}

// Manager gère les clés et les connexions SSH.
type Manager struct {
	exec    Executor
	keysDir string
}

// Option configure un Manager.
type Option func(*Manager)

// WithExecutor injecte un Executor personnalisé (utilisé par les tests).
func WithExecutor(e Executor) Option { return func(m *Manager) { m.exec = e } }

// WithKeysDir surcharge le répertoire des clés.
func WithKeysDir(dir string) Option { return func(m *Manager) { m.keysDir = dir } }

// New construit un Manager avec les valeurs par défaut, surchargeables.
func New(opts ...Option) *Manager {
	m := &Manager{exec: systemExecutor{}, keysDir: config.SSHKeysDir}
	for _, o := range opts {
		o(m)
	}
	return m
}

// keyPath renvoie le chemin de la clé privée d'un conteneur.
func (m *Manager) keyPath(name string) string {
	return filepath.Join(m.keysDir, name)
}

// PublicKeyPath renvoie le chemin de la clé publique d'un conteneur.
func (m *Manager) PublicKeyPath(name string) string {
	return m.keyPath(name) + ".pub"
}

// EnsureKey garantit l'existence d'une paire ED25519 pour le conteneur et
// renvoie le contenu de la clé publique (à injecter dans authorized_keys).
// L'opération est idempotente : une clé déjà présente est réutilisée.
func (m *Manager) EnsureKey(ctx context.Context, name string) (string, error) {
	if err := container.ValidateName(name); err != nil {
		return "", err
	}
	priv := m.keyPath(name)

	switch _, err := os.Stat(priv); {
	case err == nil:
		// Clé déjà générée : on la réutilise.
		return m.readPublicKey(name)
	case !os.IsNotExist(err):
		return "", apperrors.Wrap("ssh-key", name, err)
	}

	if err := os.MkdirAll(m.keysDir, 0o700); err != nil {
		return "", apperrors.Wrap("ssh-key", name, err)
	}

	argv := []string{
		"ssh-keygen",
		"-t", config.SSHKeyType,
		"-f", priv,
		"-N", "", // clé de service : pas de passphrase interactive
		"-C", config.AppName + ":" + name,
		"-q",
	}
	log.Audit("ssh-key", name, log.Fields{"type": config.SSHKeyType})
	if out, err := m.exec.Capture(ctx, argv); err != nil {
		return "", apperrors.Wrap("ssh-key", name, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))))
	}

	// ssh-keygen pose déjà 0600 ; on le réaffirme (defense in depth).
	if err := os.Chmod(priv, config.SSHKeyPerm); err != nil {
		return "", apperrors.Wrap("ssh-key", name, err)
	}
	return m.readPublicKey(name)
}

// readPublicKey lit et détrime le contenu de la clé publique.
func (m *Manager) readPublicKey(name string) (string, error) {
	data, err := os.ReadFile(m.PublicKeyPath(name))
	if err != nil {
		return "", apperrors.Wrap("ssh-key", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// InstallKey installe la clé publique du conteneur dans le authorized_keys du
// compte root de son rootfs, avec des permissions strictes (.ssh en 0700,
// authorized_keys en 0600). Idempotent : le fichier est réécrit.
func (m *Manager) InstallKey(name, rootfsDir string) error {
	pub, err := m.readPublicKey(name)
	if err != nil {
		return err
	}
	sshDir := filepath.Join(rootfsDir, "root", ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return apperrors.Wrap("ssh-key", name, err)
	}
	authorized := filepath.Join(sshDir, "authorized_keys")
	if err := os.WriteFile(authorized, []byte(pub+"\n"), config.SSHKeyPerm); err != nil {
		return apperrors.Wrap("ssh-key", name, err)
	}
	return nil
}

// ConnectOptions paramètre l'ouverture d'une session interactive.
type ConnectOptions struct {
	Container *container.Container
	User      string // défaut config.SSHUser si vide
	Streams   Streams
	// Password active l'authentification par mot de passe (opt-in explicite,
	// sécurité réduite). Par défaut faux : clé uniquement (rules/security.md).
	Password bool
}

// Connect ouvre une session SSH interactive durcie vers le conteneur. La paire
// de clés doit déjà exister (voir EnsureKey).
func (m *Manager) Connect(ctx context.Context, opts ConnectOptions) error {
	c := opts.Container
	if c == nil {
		return fmt.Errorf("ssh: nil container")
	}
	if err := container.ValidateName(c.Name); err != nil {
		return err
	}
	// En mode clé (défaut), la paire doit exister ; en mode mot de passe,
	// la clé est facultative (ssh demandera le mot de passe).
	if !opts.Password {
		if _, err := os.Stat(m.keyPath(c.Name)); err != nil {
			return apperrors.Wrap("ssh", c.Name,
				fmt.Errorf("clé absente (générer via EnsureKey): %w", err))
		}
	}

	user := opts.User
	if user == "" {
		user = config.SSHUser
	}

	argv := m.connectArgs(c, user, opts.Password)
	log.Audit("ssh", c.Name, log.Fields{"user": user, "port": c.SSHPort, "password_auth": opts.Password})
	return apperrors.Wrap("ssh", c.Name, m.exec.Interactive(ctx, argv, opts.Streams))
}

// connectArgs construit la ligne de commande SSH.
//
// Par défaut (password=false), durcissement imposé (rules/security.md → SSH
// Hardening) : clé uniquement (IdentitiesOnly + PreferredAuthentications=publickey),
// mot de passe refusé, pas de forwarding d'agent, port applicatif (> 10000)
// redirigé sur la loopback hôte.
//
// Avec password=true (opt-in explicite, sécurité réduite), on autorise le mot
// de passe : ssh le demandera de façon interactive. La clé reste proposée si
// elle existe. ForwardAgent=no est conservé dans les deux cas.
func (m *Manager) connectArgs(c *container.Container, user string, password bool) []string {
	argv := []string{"ssh", "-p", strconv.Itoa(c.SSHPort)}

	if password {
		argv = append(argv,
			"-o", "PubkeyAuthentication=yes",
			"-o", "PreferredAuthentications=publickey,password",
		)
		if _, err := os.Stat(m.keyPath(c.Name)); err == nil {
			argv = append(argv, "-i", m.keyPath(c.Name), "-o", "IdentitiesOnly=yes")
		}
	} else {
		argv = append(argv,
			"-i", m.keyPath(c.Name),
			"-o", "IdentitiesOnly=yes",
			"-o", "PubkeyAuthentication=yes",
			"-o", "PasswordAuthentication=no",
			"-o", "PreferredAuthentications=publickey",
		)
	}

	argv = append(argv,
		"-o", "ForwardAgent=no",
		"-o", "StrictHostKeyChecking=accept-new",
		fmt.Sprintf("%s@%s", user, config.SSHHost),
	)
	return argv
}
