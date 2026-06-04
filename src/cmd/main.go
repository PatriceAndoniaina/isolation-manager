// Command isolation-manager est le point d'entrée CLI du gestionnaire de
// conteneurs. Conformément au layering, cmd/ ne contient que le câblage des
// commandes : la logique métier vit dans pkg/.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	"github.com/PatriceAndoniaina/isolation-manager/src/internal/log"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/cgroups"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/logs"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/nginx"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/nspawn"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/ssh"
)

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
	return cmd
}
