package nspawn

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	"github.com/PatriceAndoniaina/isolation-manager/src/internal/log"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// Create provisionne le rootfs et persiste les métadonnées d'un conteneur.
//
// La population du rootfs (debootstrap / clone de template) relève d'une couche
// ultérieure : ici on garantit l'unicité du nom, l'allocation d'un port SSH et
// la création du répertoire machine.
func (m *Manager) Create(ctx context.Context, opts container.CreateOptions) (*container.Container, error) {
	if err := container.ValidateName(opts.Name); err != nil {
		return nil, err
	}
	if m.store.exists(opts.Name) {
		return nil, apperrors.Wrap("create", opts.Name, apperrors.ErrAlreadyExists)
	}

	existing, err := m.store.list()
	if err != nil {
		return nil, apperrors.Wrap("create", opts.Name, err)
	}
	port, err := allocatePort(existing)
	if err != nil {
		return nil, apperrors.Wrap("create", opts.Name, err)
	}

	limits := opts.Limits
	if limits == (container.ResourceLimits{}) {
		limits = container.DefaultLimits()
	}

	c := &container.Container{
		Name:      opts.Name,
		State:     container.StateCreated,
		SSHPort:   port,
		Limits:    limits,
		CreatedAt: time.Now().UTC(),
	}

	dir := filepath.Join(m.machinesDir, opts.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, apperrors.Wrap("create", opts.Name, err)
	}
	if err := m.store.save(c); err != nil {
		// Annule le répertoire pour ne pas laisser d'état partiel.
		_ = os.RemoveAll(dir)
		return nil, apperrors.Wrap("create", opts.Name, err)
	}

	log.Audit("create", opts.Name, log.Fields{"ssh_port": port, "memory_mb": limits.MemoryMB})
	return c, nil
}

// Start démarre un conteneur via systemd-run + systemd-nspawn, en appliquant
// les drapeaux de sécurité et les limites cgroup. Idempotent si déjà démarré.
func (m *Manager) Start(ctx context.Context, name string) error {
	c, err := m.store.load(name)
	if err != nil {
		return err
	}
	if c.State == container.StateRunning {
		return nil
	}

	ctx, cancel := withTimeout(ctx, config.StartTimeout)
	defer cancel()

	if _, err := m.exec(ctx, "start", name, m.buildStartArgs(c)); err != nil {
		return err
	}

	c.State = container.StateRunning
	if err := m.store.save(c); err != nil {
		return apperrors.Wrap("start", name, err)
	}
	return nil
}

// Stop arrête gracieusement un conteneur (poweroff), avec repli sur terminate
// si l'arrêt propre dépasse le délai imparti.
func (m *Manager) Stop(ctx context.Context, name string) error {
	c, err := m.store.load(name)
	if err != nil {
		return err
	}

	sctx, cancel := withTimeout(ctx, config.StopTimeout)
	defer cancel()
	if _, err := m.exec(sctx, "stop", name, []string{"machinectl", "poweroff", name}); err != nil {
		// Repli : arrêt forcé (équivalent SIGKILL).
		fctx, fcancel := withTimeout(ctx, config.StopTimeout)
		defer fcancel()
		if _, ferr := m.exec(fctx, "stop", name, []string{"machinectl", "terminate", name}); ferr != nil {
			return err
		}
	}

	c.State = container.StateStopped
	if err := m.store.save(c); err != nil {
		return apperrors.Wrap("stop", name, err)
	}
	return nil
}

// Destroy arrête (best-effort), supprime le rootfs et les métadonnées.
func (m *Manager) Destroy(ctx context.Context, name string) error {
	if !m.store.exists(name) {
		return apperrors.Wrap("destroy", name, apperrors.ErrNotFound)
	}

	tctx, cancel := withTimeout(ctx, config.StopTimeout)
	defer cancel()
	// Best-effort : un conteneur déjà arrêté fait échouer terminate, sans gravité.
	_, _ = m.exec(tctx, "destroy", name, []string{"machinectl", "terminate", name})

	if err := os.RemoveAll(filepath.Join(m.machinesDir, name)); err != nil {
		return apperrors.Wrap("destroy", name, err)
	}
	if err := m.store.delete(name); err != nil {
		return err
	}

	log.Audit("destroy", name, nil)
	return nil
}

// buildStartArgs construit la ligne de commande complète du démarrage.
//
// Forme : systemd-run (limites cgroup en propriétés) → systemd-nspawn (boot du
// conteneur avec rootfs en lecture seule, tmpfs et port SSH exposé). Aucun
// drapeau privilégié n'est ajouté ; le garde-fou rejetterait toute tentative.
func (m *Manager) buildStartArgs(c *container.Container) []string {
	argv := []string{
		"systemd-run",
		"--unit=" + config.UnitName(c.Name),
		"--collect",
		fmt.Sprintf("--property=MemoryMax=%dM", c.Limits.MemoryMB),
		fmt.Sprintf("--property=CPUQuota=%d%%", c.Limits.CPUQuota),
		fmt.Sprintf("--property=TasksMax=%d", c.Limits.PidsLimit),
		"systemd-nspawn",
		"--quiet",
		"--boot",
		fmt.Sprintf("--machine=%s", c.Name),
		fmt.Sprintf("--directory=%s", filepath.Join(m.machinesDir, c.Name)),
		"--network-veth",
		fmt.Sprintf("--port=%d:22", c.SSHPort),
	}
	// Drapeaux de sécurité imposés (rootfs read-only + tmpfs durcis).
	argv = append(argv, config.SecurityFlags...)
	return argv
}

// allocatePort attribue le plus petit port SSH libre dans la plage autorisée.
func allocatePort(existing []*container.Container) (int, error) {
	used := make(map[int]bool, len(existing))
	for _, c := range existing {
		used[c.SSHPort] = true
	}
	for p := config.SSHPortMin; p <= config.SSHPortMax; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free SSH port in range %d-%d", config.SSHPortMin, config.SSHPortMax)
}
