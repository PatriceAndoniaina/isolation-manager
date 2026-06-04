// Package container définit le modèle métier d'un conteneur et l'interface
// Containerizer que les backends (systemd-nspawn, mocks de test) implémentent.
//
// Ce package est le cœur du domaine : il ne dépend que de la stdlib et des
// packages config/errors. Les implémentations concrètes (pkg/nspawn) en
// dépendront, jamais l'inverse (règle : dépendances basées sur interfaces).
package container

import (
	"context"
	"regexp"
	"time"

	"github.com/PatriceAndoniaina/isolation-manager/src/config"
	apperrors "github.com/PatriceAndoniaina/isolation-manager/src/pkg/errors"
)

// State représente l'état du cycle de vie d'un conteneur.
type State string

const (
	StateUnknown   State = "unknown"
	StateCreated   State = "created"
	StateRunning   State = "running"
	StateStopped   State = "stopped"
	StateDestroyed State = "destroyed"
)

// nameRE valide les noms de conteneurs : lettres minuscules, chiffres et
// tirets, commençant par une lettre (compatible noms de machine systemd).
var nameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

// ResourceLimits décrit les limites cgroup appliquées à un conteneur.
type ResourceLimits struct {
	MemoryMB  int // limite mémoire en mégaoctets
	CPUQuota  int // quota CPU en pourcentage (100 = 1 cœur)
	PidsLimit int // nombre maximal de processus
}

// DefaultLimits renvoie les limites par défaut issues de la config.
func DefaultLimits() ResourceLimits {
	return ResourceLimits{
		MemoryMB:  config.DefaultMemoryLimitMB,
		CPUQuota:  config.DefaultCPUQuota,
		PidsLimit: config.DefaultPidsLimit,
	}
}

// Container est la représentation métier d'un conteneur isolé.
type Container struct {
	Name      string         // identifiant unique (= nom de machine systemd)
	State     State          // état courant
	SSHPort   int            // port SSH exposé (> 10000)
	Limits    ResourceLimits // limites de ressources appliquées
	CreatedAt time.Time      // horodatage de création
}

// CreateOptions regroupe les paramètres de création d'un conteneur.
type CreateOptions struct {
	Name   string
	Limits ResourceLimits
}

// Containerizer est le contrat que tout backend de conteneurisation doit
// respecter. context.Context est propagé partout pour porter les timeouts
// et l'annulation (règle de concurrence).
type Containerizer interface {
	Create(ctx context.Context, opts CreateOptions) (*Container, error)
	Start(ctx context.Context, name string) error
	Stop(ctx context.Context, name string) error
	Destroy(ctx context.Context, name string) error
	List(ctx context.Context) ([]*Container, error)
	Get(ctx context.Context, name string) (*Container, error)
}

// ValidateName vérifie qu'un nom de conteneur respecte le format attendu.
func ValidateName(name string) error {
	if !nameRE.MatchString(name) {
		return apperrors.Wrap("validate", name, apperrors.ErrInvalidName)
	}
	return nil
}
