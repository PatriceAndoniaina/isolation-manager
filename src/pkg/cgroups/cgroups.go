// Package cgroups lit les compteurs de ressources d'un conteneur directement
// dans la hiérarchie cgroup v2 (sysfs).
//
// Le conteneur est démarré par pkg/nspawn via une unité systemd transitoire
// (systemd-run --unit=<config.UnitName>) : ses cgroups vivent donc sous
// <root>/<slice>/<unit>.service. La lecture est purement filesystem (aucun
// appel système externe), ce qui la rend rapide (cible : resource_read < 50ms)
// et testable en injectant une fausse racine cgroup via WithRoot.
package cgroups

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

const (
	// defaultRoot est le point de montage standard de cgroup v2.
	defaultRoot = "/sys/fs/cgroup"
	// defaultSlice est la tranche systemd hébergeant les unités transitoires
	// créées par systemd-run (mode système).
	defaultSlice = "system.slice"
	// unitSuffix : systemd-run produit une unité de type service.
	unitSuffix = ".service"
)

// Stats est un instantané des compteurs cgroup d'un conteneur.
type Stats struct {
	MemoryCurrentBytes uint64    // mémoire utilisée (memory.current)
	MemoryMaxBytes     uint64    // limite mémoire ; 0 = illimité ("max")
	CPUUsageUsec       uint64    // temps CPU cumulé en µs (cpu.stat usage_usec)
	PidsCurrent        uint64    // processus actifs (pids.current)
	PidsMax            uint64    // limite de processus ; 0 = illimité ("max")
	ReadAt             time.Time // horodatage de la lecture (pour le calcul CPU%)
}

// Reader lit les statistiques cgroup. La racine et la tranche sont
// configurables pour permettre l'injection d'un arbre factice dans les tests.
type Reader struct {
	root  string
	slice string
}

// Option configure un Reader.
type Option func(*Reader)

// WithRoot surcharge la racine cgroup (par défaut /sys/fs/cgroup).
func WithRoot(dir string) Option { return func(r *Reader) { r.root = dir } }

// WithSlice surcharge la tranche systemd parente (par défaut system.slice).
func WithSlice(s string) Option { return func(r *Reader) { r.slice = s } }

// NewReader construit un Reader avec les valeurs par défaut, surchargeables.
func NewReader(opts ...Option) *Reader {
	r := &Reader{root: defaultRoot, slice: defaultSlice}
	for _, o := range opts {
		o(r)
	}
	return r
}

// unitDir renvoie le répertoire cgroup de l'unité associée au conteneur.
func (r *Reader) unitDir(name string) string {
	return filepath.Join(r.root, r.slice, config.UnitName(name)+unitSuffix)
}

// Read renvoie un instantané des compteurs cgroup du conteneur. Si le
// répertoire cgroup est absent, le conteneur n'est pas démarré : ErrNotRunning.
func (r *Reader) Read(ctx context.Context, name string) (Stats, error) {
	if err := ctx.Err(); err != nil {
		return Stats{}, apperrors.Wrap("stats", name, err)
	}

	dir := r.unitDir(name)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return Stats{}, apperrors.Wrap("stats", name, apperrors.ErrNotRunning)
		}
		return Stats{}, apperrors.Wrap("stats", name, err)
	}

	var (
		s   Stats
		err error
	)
	if s.MemoryCurrentBytes, err = readUint(dir, "memory.current"); err != nil {
		return Stats{}, apperrors.Wrap("stats", name, err)
	}
	if s.MemoryMaxBytes, err = readLimit(dir, "memory.max"); err != nil {
		return Stats{}, apperrors.Wrap("stats", name, err)
	}
	if s.CPUUsageUsec, err = readCPUUsage(dir); err != nil {
		return Stats{}, apperrors.Wrap("stats", name, err)
	}
	if s.PidsCurrent, err = readUint(dir, "pids.current"); err != nil {
		return Stats{}, apperrors.Wrap("stats", name, err)
	}
	if s.PidsMax, err = readLimit(dir, "pids.max"); err != nil {
		return Stats{}, apperrors.Wrap("stats", name, err)
	}
	s.ReadAt = time.Now()
	return s, nil
}

// Sample lit deux instantanés espacés de interval et renvoie le second, enrichi
// du pourcentage d'utilisation CPU mesuré sur l'intervalle. Si interval <= 0,
// un seul instantané est pris et le CPU% vaut 0 (faute de fenêtre de mesure).
func (r *Reader) Sample(ctx context.Context, name string, interval time.Duration) (Stats, float64, error) {
	first, err := r.Read(ctx, name)
	if err != nil {
		return Stats{}, 0, err
	}
	if interval <= 0 {
		return first, 0, nil
	}
	if err := sleep(ctx, interval); err != nil {
		return Stats{}, 0, apperrors.Wrap("stats", name, err)
	}
	second, err := r.Read(ctx, name)
	if err != nil {
		return Stats{}, 0, err
	}
	return second, CPUPercent(first, second), nil
}

// CPUPercent calcule l'utilisation CPU entre deux instantanés.
// 100% correspond à un cœur pleinement occupé ; la valeur peut dépasser 100%
// sur plusieurs cœurs. Les deltas négatifs ou nuls renvoient 0.
func CPUPercent(prev, cur Stats) float64 {
	wall := cur.ReadAt.Sub(prev.ReadAt)
	if wall <= 0 || cur.CPUUsageUsec < prev.CPUUsageUsec {
		return 0
	}
	deltaCPU := time.Duration(cur.CPUUsageUsec-prev.CPUUsageUsec) * time.Microsecond
	return float64(deltaCPU) / float64(wall) * 100
}

// sleep attend d en respectant l'annulation du contexte.
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// readField lit un fichier cgroup et renvoie son contenu détrimé.
func readField(dir, file string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// readUint lit un compteur entier non signé.
func readUint(dir, file string) (uint64, error) {
	s, err := readField(dir, file)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(s, 10, 64)
}

// readLimit lit une limite cgroup où la valeur littérale "max" (illimité) est
// traduite en 0.
func readLimit(dir, file string) (uint64, error) {
	s, err := readField(dir, file)
	if err != nil {
		return 0, err
	}
	if s == "max" {
		return 0, nil
	}
	return strconv.ParseUint(s, 10, 64)
}

// readCPUUsage extrait le champ usage_usec du fichier cpu.stat.
func readCPUUsage(dir string) (uint64, error) {
	data, err := os.ReadFile(filepath.Join(dir, "cpu.stat"))
	if err != nil {
		return 0, err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[0] == "usage_usec" {
			return strconv.ParseUint(fields[1], 10, 64)
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, fmt.Errorf("usage_usec absent de cpu.stat")
}
