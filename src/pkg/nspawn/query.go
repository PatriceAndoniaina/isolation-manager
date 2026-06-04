package nspawn

import (
	"context"
	"strings"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	"github.com/PatriceAndoniaina/isolation-manager/src/pkg/container"
)

// List renvoie tous les conteneurs connus, d'après l'état persisté.
func (m *Manager) List(ctx context.Context) ([]*container.Container, error) {
	return m.store.list()
}

// Get renvoie un conteneur et rafraîchit son état d'exécution réel via
// machinectl (le store peut être en retard si le conteneur s'est arrêté seul).
func (m *Manager) Get(ctx context.Context, name string) (*container.Container, error) {
	c, err := m.store.load(name)
	if err != nil {
		return nil, err
	}

	running := m.isRunning(ctx, name)
	switch {
	case running:
		c.State = container.StateRunning
	case c.State == container.StateRunning:
		// Le store le croyait actif mais il ne l'est plus.
		c.State = container.StateStopped
	}
	return c, nil
}

// isRunning interroge machinectl pour savoir si la machine tourne. Toute erreur
// (machine inconnue de machined, non démarrée) est interprétée comme "arrêté".
func (m *Manager) isRunning(ctx context.Context, name string) bool {
	rctx, cancel := withTimeout(ctx, config.DefaultExecTimeout)
	defer cancel()

	out, err := m.exec(rctx, "get", name,
		[]string{"machinectl", "show", name, "-p", "State", "--value"})
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "running"
}
