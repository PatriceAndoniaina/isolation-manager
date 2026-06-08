// Command isolation-manager est le point d'entrée CLI du gestionnaire de
// conteneurs. Conformément au layering, cmd/ ne contient que le câblage des
// commandes : la logique métier vit dans pkg/.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	"github.com/PatriceAndoniaina/isolation-manager/src/internal/log"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/cgroups"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/deploy"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/logs"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/nginx"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/nspawn"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/preflight"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/ssh"
)

// opDeps liste les binaires externes que chaque commande exécute réellement.
// Le preflight les vérifie (et les installe si --auto-install) avant l'action.
var opDeps = map[string][]string{
	"start":  {"systemd-run", "systemd-nspawn"},
	"stop":   {"machinectl"},
	"ssh":    {"ssh", "ssh-keygen"},
	"logs":   {"journalctl"},
	"deploy": {"ssh", "rsync", "go"},
	"remote": {"ssh"},
}

// ensureDeps lance le preflight pour l'opération op (no-op si rien à vérifier).
func ensureDeps(cmd *cobra.Command, op string) error {
	deps := opDeps[op]
	if len(deps) == 0 {
		return nil
	}
	auto, _ := cmd.Flags().GetBool("auto-install")
	return preflight.New().Ensure(cmd.Context(), cmd.ErrOrStderr(), deps, auto)
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		// Cobra a déjà affiché l'erreur ; on sort en échec.
		os.Exit(1)
	}
}

// newRootCmd construit la commande racine et ses sous-commandes.
func newRootCmd() *cobra.Command {
	var (
		verbose  bool
		jsonLogs bool
	)

	// Backend de conteneurisation injecté dans les commandes (interface).
	var mgr container.Containerizer = nspawn.New()

	root := &cobra.Command{
		Use:     config.AppName,
		Short:   "Gestionnaire d'isolation utilisateur par conteneurs systemd-nspawn",
		Version: config.Version,
		// SilenceUsage évite d'afficher l'aide complète sur une erreur runtime.
		SilenceUsage: true,
		PersistentPreRun: func(_ *cobra.Command, _ []string) {
			if verbose {
				log.SetLevel(logrus.DebugLevel)
			}
			if jsonLogs {
				log.UseJSON()
			}
		},
	}

	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "logs détaillés (debug)")
	root.PersistentFlags().BoolVar(&jsonLogs, "json", false, "logs au format JSON")
	root.PersistentFlags().Bool("auto-install", true,
		"installer automatiquement les dépendances manquantes (apt/dnf/pacman)")

	root.AddCommand(
		newCreateCmd(mgr),
		newListCmd(mgr),
		newStartCmd(mgr),
		newStopCmd(mgr),
		newDestroyCmd(mgr),
		newSSHCmd(mgr),
		newLogsCmd(mgr),
		newStatsCmd(mgr),
		newNginxCmd(mgr),
		newDeployCmd(),
		newRemoteCmd(),
	)
	return root
}

// newCreateCmd : création d'un conteneur isolé.
func newCreateCmd(mgr container.Containerizer) *cobra.Command {
	limits := container.DefaultLimits()
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Créer un nouveau conteneur isolé",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := mgr.Create(cmd.Context(), container.CreateOptions{
				Name:   args[0],
				Limits: limits,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"✅ conteneur %q créé (ssh port %d, mémoire %d Mo)\n",
				c.Name, c.SSHPort, c.Limits.MemoryMB)
			return nil
		},
	}
	cmd.Flags().IntVar(&limits.MemoryMB, "memory", limits.MemoryMB, "limite mémoire (Mo)")
	cmd.Flags().IntVar(&limits.CPUQuota, "cpu", limits.CPUQuota, "quota CPU (%)")
	cmd.Flags().IntVar(&limits.PidsLimit, "pids", limits.PidsLimit, "nombre max de processus")
	return cmd
}

// newListCmd : liste les conteneurs existants.
func newListCmd(mgr container.Containerizer) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "Lister les conteneurs",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := mgr.List(cmd.Context())
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "NAME\tSTATE\tSSH\tMEM(MB)")
			for _, c := range list {
				fmt.Fprintf(tw, "%s\t%s\t%d\t%d\n", c.Name, c.State, c.SSHPort, c.Limits.MemoryMB)
			}
			return tw.Flush()
		},
	}
}

// newStartCmd : démarrage d'un conteneur (applique limites + drapeaux sécurité).
func newStartCmd(mgr container.Containerizer) *cobra.Command {
	return &cobra.Command{
		Use:   "start <name>",
		Short: "Démarrer un conteneur",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureDeps(cmd, "start"); err != nil {
				return err
			}
			if err := mgr.Start(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "▶️  conteneur %q démarré\n", args[0])
			return nil
		},
	}
}

// newStopCmd : arrêt gracieux d'un conteneur (repli sur terminate si besoin).
func newStopCmd(mgr container.Containerizer) *cobra.Command {
	return &cobra.Command{
		Use:   "stop <name>",
		Short: "Arrêter un conteneur",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureDeps(cmd, "stop"); err != nil {
				return err
			}
			if err := mgr.Stop(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "⏹️  conteneur %q arrêté\n", args[0])
			return nil
		},
	}
}

// newDestroyCmd : suppression d'un conteneur.
func newDestroyCmd(mgr container.Containerizer) *cobra.Command {
	return &cobra.Command{
		Use:     "destroy <name>",
		Aliases: []string{"rm"},
		Short:   "Détruire un conteneur",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := mgr.Destroy(cmd.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "🗑️  conteneur %q détruit\n", args[0])
			return nil
		},
	}
}

// newSSHCmd : ouverture d'une session SSH durcie vers un conteneur.
func newSSHCmd(mgr container.Containerizer) *cobra.Command {
	var user string
	cmd := &cobra.Command{
		Use:   "ssh <name>",
		Short: "Se connecter en SSH à un conteneur",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := container.ValidateName(name); err != nil {
				return err
			}
			if err := ensureDeps(cmd, "ssh"); err != nil {
				return err
			}
			c, err := mgr.Get(cmd.Context(), name)
			if err != nil {
				return err
			}
			if c.State != container.StateRunning {
				return apperrors.Wrap("ssh", name, apperrors.ErrNotRunning)
			}

			sshMgr := ssh.New()
			// Garantit la paire de clés puis l'injecte dans le rootfs du conteneur.
			if _, err := sshMgr.EnsureKey(cmd.Context(), name); err != nil {
				return err
			}
			if err := sshMgr.InstallKey(name, filepath.Join(config.MachinesDir, name)); err != nil {
				return err
			}
			return sshMgr.Connect(cmd.Context(), ssh.ConnectOptions{
				Container: c,
				User:      user,
				Streams: ssh.Streams{
					In:  cmd.InOrStdin(),
					Out: cmd.OutOrStdout(),
					Err: cmd.ErrOrStderr(),
				},
			})
		},
	}
	cmd.Flags().StringVarP(&user, "user", "u", config.SSHUser, "utilisateur SSH cible")
	return cmd
}

// newLogsCmd : consultation des journaux d'un conteneur (journald).
func newLogsCmd(mgr container.Containerizer) *cobra.Command {
	var (
		follow bool
		lines  int
		since  string
	)
	cmd := &cobra.Command{
		Use:   "logs <name>",
		Short: "Afficher les journaux d'un conteneur",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := container.ValidateName(name); err != nil {
				return err
			}
			if err := ensureDeps(cmd, "logs"); err != nil {
				return err
			}
			// Confirme l'existence du conteneur (les journaux restent
			// disponibles même s'il est arrêté).
			if _, err := mgr.Get(cmd.Context(), name); err != nil {
				return err
			}

			// Ctrl-C interrompt proprement le mode --follow.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			err := logs.New().Fetch(ctx, cmd.OutOrStdout(), logs.Options{
				Name:   name,
				Lines:  lines,
				Since:  since,
				Follow: follow,
			})
			if follow && errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "suivre les journaux en continu")
	cmd.Flags().IntVarP(&lines, "lines", "n", config.DefaultLogLines, "nombre de lignes à afficher")
	cmd.Flags().StringVar(&since, "since", "", "afficher depuis (ex: \"1h ago\", \"2024-01-01\")")
	return cmd
}

// newStatsCmd : métriques temps réel d'un conteneur (cgroups v2).
func newStatsCmd(mgr container.Containerizer) *cobra.Command {
	var (
		watch    bool
		interval time.Duration
	)
	cmd := &cobra.Command{
		Use:   "stats <name>",
		Short: "Afficher les métriques d'un conteneur (cgroups)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := container.ValidateName(name); err != nil {
				return err
			}
			// Vérifie l'existence et récupère les limites configurées
			// (servent de plafond d'affichage si le cgroup dit "max").
			c, err := mgr.Get(cmd.Context(), name)
			if err != nil {
				return err
			}

			// Ctrl-C interrompt proprement le mode --watch.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			reader := cgroups.NewReader()
			show := func() error {
				s, cpu, err := reader.Sample(ctx, name, interval)
				if err != nil {
					return err
				}
				writeStats(cmd.OutOrStdout(), c, s, cpu)
				return nil
			}

			if !watch {
				return show()
			}
			for {
				if err := show(); err != nil {
					if errors.Is(err, context.Canceled) {
						return nil
					}
					return err
				}
				if ctx.Err() != nil {
					return nil
				}
			}
		},
	}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "rafraîchir en continu (Ctrl-C pour quitter)")
	cmd.Flags().DurationVar(&interval, "interval", 500*time.Millisecond,
		"fenêtre de mesure du CPU%")
	return cmd
}

// writeStats formate un instantané de métriques sous forme de tableau.
// Lorsque le cgroup ne fixe pas de limite ("max" → 0), on retombe sur la limite
// configurée du conteneur pour donner un repère à l'utilisateur.
func writeStats(w io.Writer, c *container.Container, s cgroups.Stats, cpu float64) {
	memMax := s.MemoryMaxBytes
	if memMax == 0 {
		memMax = uint64(c.Limits.MemoryMB) * 1024 * 1024
	}
	pidsMax := s.PidsMax
	if pidsMax == 0 {
		pidsMax = uint64(c.Limits.PidsLimit)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tCPU%\tMEM\tPIDS")
	fmt.Fprintf(tw, "%s\t%.1f\t%s / %s\t%d / %d\n",
		c.Name, cpu,
		humanBytes(s.MemoryCurrentBytes), humanBytes(memMax),
		s.PidsCurrent, pidsMax)
	tw.Flush()
}

// newDeployCmd : déploiement de l'outil sur un serveur Linux distant.
func newDeployCmd() *cobra.Command {
	var (
		host, user, key, remotePath, source string
		port                                int
		installDeps                         bool
	)
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Déployer l'outil sur un serveur Linux distant (SSH + rsync)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := ensureDeps(cmd, "deploy"); err != nil {
				return err
			}
			return deploy.New().Deploy(cmd.Context(), cmd.OutOrStdout(), deploy.Options{
				Target:      deploy.Target{Host: host, User: user, Port: port, Key: key},
				RemotePath:  remotePath,
				Source:      source,
				InstallDeps: installDeps,
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&host, "host", "", "hôte du serveur (requis)")
	f.StringVarP(&user, "user", "u", "root", "utilisateur SSH")
	f.IntVarP(&port, "port", "p", 0, "port SSH (défaut 22)")
	f.StringVarP(&key, "key", "i", "", "clé privée SSH")
	f.StringVar(&remotePath, "remote-path", config.RemoteBinPath, "chemin d'installation distant")
	f.StringVar(&source, "source", ".", "racine du module à compiler")
	f.BoolVar(&installDeps, "install-deps", true, "installer systemd-container si absent")
	_ = cmd.MarkFlagRequired("host")
	return cmd
}

// newRemoteCmd : exécution d'une sous-commande sur le serveur distant via SSH.
// Les flags doivent précéder la sous-commande : `remote --host srv create user01`.
func newRemoteCmd() *cobra.Command {
	var (
		host, user, key, remotePath string
		port                        int
	)
	cmd := &cobra.Command{
		Use:   "remote --host <srv> <command> [args...]",
		Short: "Exécuter une commande de l'outil sur le serveur distant",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ensureDeps(cmd, "remote"); err != nil {
				return err
			}
			return deploy.New().Exec(cmd.Context(),
				deploy.Target{Host: host, User: user, Port: port, Key: key},
				remotePath, args,
				deploy.Streams{In: cmd.InOrStdin(), Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()})
		},
	}
	f := cmd.Flags()
	// Tout ce qui suit le premier argument positionnel est transmis au serveur
	// (et non interprété comme flag local).
	f.SetInterspersed(false)
	f.StringVar(&host, "host", "", "hôte du serveur (requis)")
	f.StringVarP(&user, "user", "u", "root", "utilisateur SSH")
	f.IntVarP(&port, "port", "p", 0, "port SSH (défaut 22)")
	f.StringVarP(&key, "key", "i", "", "clé privée SSH")
	f.StringVar(&remotePath, "remote-path", config.RemoteBinPath, "chemin du binaire distant")
	_ = cmd.MarkFlagRequired("host")
	return cmd
}

// humanBytes formate un nombre d'octets en unités binaires lisibles (KiB, MiB…).
func humanBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// newNginxCmd : génération de la configuration nginx (reverse proxy) durcie.
func newNginxCmd(mgr container.Containerizer) *cobra.Command {
	var serverName, upstream, certPath, keyPath, outPath string
	cmd := &cobra.Command{
		Use:   "nginx <name>",
		Short: "Générer la configuration nginx (reverse proxy) d'un conteneur",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := container.ValidateName(name); err != nil {
				return err
			}
			if _, err := mgr.Get(cmd.Context(), name); err != nil {
				return err
			}

			conf, err := nginx.NewGenerator().Render(nginx.SiteOptions{
				Name:        name,
				ServerName:  serverName,
				Upstream:    upstream,
				TLSCertPath: certPath,
				TLSKeyPath:  keyPath,
			})
			if err != nil {
				return err
			}

			if outPath == "" {
				fmt.Fprint(cmd.OutOrStdout(), conf)
				return nil
			}
			if err := os.WriteFile(outPath, []byte(conf), 0o644); err != nil {
				return apperrors.Wrap("nginx", name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ configuration nginx écrite dans %s\n", outPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&serverName, "server-name", "", "nom de domaine servi (requis)")
	cmd.Flags().StringVar(&upstream, "upstream", "", "adresse du conteneur host:port (requis)")
	cmd.Flags().StringVar(&certPath, "tls-cert", "", "chemin du certificat TLS (requis)")
	cmd.Flags().StringVar(&keyPath, "tls-key", "", "chemin de la clé TLS (requis)")
	cmd.Flags().StringVarP(&outPath, "output", "o", "", "fichier de sortie (défaut: stdout)")
	for _, f := range []string{"server-name", "upstream", "tls-cert", "tls-key"} {
		_ = cmd.MarkFlagRequired(f)
	}
	cmd.AddCommand(newNginxFmtCmd(), newNginxValidateCmd(), newNginxListCmd(), newNginxRmCmd())
	return cmd
}

// newNginxRmCmd : supprime un fichier de configuration nginx (*.conf), avec
// confirmation (sautée par -f) et garde-fou sur l'extension.
func newNginxRmCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:     "rm <fichier>",
		Aliases: []string{"delete"},
		Short:   "Supprimer un fichier de configuration nginx (*.conf)",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if !nginx.IsConfigFile(path) {
				return fmt.Errorf("refus de suppression : %q n'est pas un fichier .conf", path)
			}
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return fmt.Errorf("%q est un dossier, pas un fichier", path)
			}
			if !force {
				fmt.Fprintf(cmd.OutOrStdout(), "Supprimer %s ? [y/N] ", path)
				line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				switch strings.ToLower(strings.TrimSpace(line)) {
				case "y", "yes", "o", "oui":
				default:
					fmt.Fprintln(cmd.OutOrStdout(), "annulé")
					return nil
				}
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "🗑️  %s supprimé\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&force, "force", "f", false, "supprimer sans confirmation")
	return cmd
}

// newNginxListCmd : liste les fichiers de configuration nginx (*.conf) d'un
// dossier, avec validation optionnelle (syntaxe + sécurité) de chacun.
func newNginxListCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "list <dossier>",
		Short: "Lister les fichiers nginx (*.conf) d'un dossier",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := nginx.ListFiles(args[0])
			if err != nil {
				return err
			}
			if len(files) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "aucun fichier .conf trouvé")
				return nil
			}
			if !check {
				for _, f := range files {
					fmt.Fprintln(cmd.OutOrStdout(), f)
				}
				return nil
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "FICHIER\tSTATUT")
			for _, f := range files {
				fmt.Fprintf(tw, "%s\t%s\n", f, nginxFileStatus(f))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVarP(&check, "check", "c", false, "valider chaque fichier (syntaxe + sécurité)")
	return cmd
}

// nginxFileStatus renvoie un statut lisible pour un fichier nginx.
func nginxFileStatus(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "⚠️  illisible: " + err.Error()
	}
	dirs, err := nginx.Parse(string(data))
	if err != nil {
		return "❌ syntaxe: " + err.Error()
	}
	if err := nginx.Validate(dirs); err != nil {
		return "⚠️  sécurité: " + err.Error()
	}
	return "✅ OK"
}

// newNginxValidateCmd : valide la syntaxe et les règles de sécurité d'un
// fichier nginx (TLS imposé, rate limit, limite de taille, pas d'upstream hôte).
func newNginxValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate <fichier>",
		Short: "Valider la syntaxe et la sécurité d'un fichier nginx",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			dirs, err := nginx.Parse(string(data))
			if err != nil {
				return apperrors.Wrap("nginx validate", path, err)
			}
			if err := nginx.Validate(dirs); err != nil {
				return apperrors.Wrap("nginx validate", path, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ %s : syntaxe et règles de sécurité OK\n", path)
			return nil
		},
	}
}

// newNginxFmtCmd : reformate un fichier de configuration nginx (à la gofmt).
func newNginxFmtCmd() *cobra.Command {
	var write bool
	cmd := &cobra.Command{
		Use:   "fmt <fichier>",
		Short: "Reformater un fichier de configuration nginx",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			dirs, err := nginx.Parse(string(data))
			if err != nil {
				return apperrors.Wrap("nginx fmt", path, err)
			}
			out := nginx.Format(dirs)

			if !write {
				fmt.Fprint(cmd.OutOrStdout(), out)
				return nil
			}
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if err := os.WriteFile(path, []byte(out), info.Mode().Perm()); err != nil {
				return apperrors.Wrap("nginx fmt", path, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✅ %s reformaté\n", path)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&write, "write", "w", false, "réécrire le fichier sur place")
	return cmd
}
