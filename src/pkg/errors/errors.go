// Package errors définit les erreurs métier du gestionnaire de conteneurs.
//
// Règle d'architecture : toutes les erreurs custom vivent ici et sont
// toujours enveloppées avec du contexte via fmt.Errorf("...: %w", err).
// Le code appelant utilise errors.Is / errors.As (stdlib) pour les inspecter.
package errors

import (
	"errors"
	"fmt"
)

// Sentinelles réutilisables, comparables avec errors.Is.
var (
	// ErrNotFound est retournée lorsqu'un conteneur n'existe pas.
	ErrNotFound = errors.New("container not found")
	// ErrAlreadyExists est retournée lors d'une création en doublon.
	ErrAlreadyExists = errors.New("container already exists")
	// ErrInvalidName signale un nom de conteneur invalide.
	ErrInvalidName = errors.New("invalid container name")
	// ErrInvalidState signale une transition d'état interdite.
	ErrInvalidState = errors.New("invalid container state")
	// ErrNotRunning signale qu'une opération exige un conteneur démarré
	// (ex: lecture des métriques cgroup d'une machine arrêtée).
	ErrNotRunning = errors.New("container is not running")
	// ErrTimeout signale le dépassement d'un timeout d'opération système.
	ErrTimeout = errors.New("operation timed out")
	// ErrPermission signale un défaut de privilèges (ex: cgroups, /var/lib/machines).
	ErrPermission = errors.New("insufficient permissions")
	// ErrNotImplemented marque les fonctionnalités encore au stade squelette.
	ErrNotImplemented = errors.New("not implemented")
	// ErrSecurityViolation signale une opération interdite par les security gates.
	ErrSecurityViolation = errors.New("security policy violation")
)

// ContainerError enveloppe une erreur en l'associant à un conteneur et une
// opération, pour produire des messages d'audit exploitables.
type ContainerError struct {
	Op        string // opération en cours (ex: "create", "start")
	Container string // nom du conteneur concerné
	Err       error  // erreur sous-jacente (enveloppée avec %w)
}

// Error implémente l'interface error.
func (e *ContainerError) Error() string {
	if e.Container == "" {
		return fmt.Sprintf("%s: %v", e.Op, e.Err)
	}
	return fmt.Sprintf("%s %q: %v", e.Op, e.Container, e.Err)
}

// Unwrap permet à errors.Is / errors.As de traverser l'enveloppe.
func (e *ContainerError) Unwrap() error { return e.Err }

// Wrap construit une *ContainerError. Retourne nil si err est nil, pour
// rester transparent dans les chaînes d'appel.
func Wrap(op, container string, err error) error {
	if err == nil {
		return nil
	}
	return &ContainerError{Op: op, Container: container, Err: err}
}

// Is réexporte errors.Is pour éviter aux appelants d'importer deux packages
// "errors" (celui-ci et la stdlib) simultanément.
func Is(err, target error) bool { return errors.Is(err, target) }

// As réexporte errors.As pour la même raison.
func As(err error, target any) bool { return errors.As(err, target) }
