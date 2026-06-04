// Package config centralise les constantes et valeurs par défaut du projet.
//
// Conformément aux règles d'architecture (cmd → pkg → std/third-party),
// ce package ne dépend d'aucun autre package interne : il n'expose que des
// constantes, des valeurs par défaut et des types de configuration simples.
package config

import "time"

// UnitName renvoie le nom de l'unité systemd transitoire associée à un
// conteneur. C'est la source de vérité unique partagée par le démarrage
// (systemd-run, pkg/nspawn) et la lecture des cgroups (pkg/cgroups) : les deux
// doivent désigner exactement la même unité.
func UnitName(container string) string {
	return AppName + "-" + container
}

// Métadonnées applicatives.
const (
	// AppName est le nom du binaire et du préfixe des conteneurs.
	AppName = "isolation-manager"
	// Version suit le versioning sémantique (voir .claude/CLAUDE.md).
	Version = "1.0.0"
)

// Chemins système utilisés par le gestionnaire de conteneurs.
//
// Aucun chemin ne doit être codé en dur ailleurs dans le code : toute
// référence passe par ces constantes (règle nspawn-expert : "no hardcoding paths").
const (
	// MachinesDir est l'emplacement standard des rootfs systemd-nspawn.
	MachinesDir = "/var/lib/machines"
	// StateDir contient l'état applicatif (métadonnées des conteneurs).
	StateDir = "/var/lib/" + AppName
	// RunDir contient les fichiers runtime (sockets, pid).
	RunDir = "/run/" + AppName
	// LogDir contient les journaux applicatifs (audit trail).
	LogDir = "/var/log/" + AppName
	// RemoteBinPath est le chemin d'installation par défaut du binaire sur un
	// serveur distant (voir pkg/deploy).
	RemoteBinPath = "/usr/local/bin/" + AppName
)

// Timeouts appliqués aux opérations système.
//
// Toute commande exec.CommandContext vers systemd-nspawn/systemctl DOIT
// utiliser un de ces timeouts (security gate : "no exec without timeout").
const (
	// DefaultExecTimeout est le timeout par défaut des appels système.
	DefaultExecTimeout = 30 * time.Second
	// CreateTimeout borne la création d'un conteneur.
	CreateTimeout = 60 * time.Second
	// StartTimeout borne le démarrage d'un conteneur.
	StartTimeout = 20 * time.Second
	// StopTimeout borne l'arrêt gracieux (SIGTERM) avant SIGKILL.
	StopTimeout = 5 * time.Second
)

// Cibles de performance (voir performance_targets dans .claude/CLAUDE.md).
// Utilisées par le monitoring pour signaler les dépassements de budget.
const (
	TargetCreate      = 500 * time.Millisecond
	TargetStart       = 200 * time.Millisecond
	TargetStop        = 100 * time.Millisecond
	TargetResourceGet = 50 * time.Millisecond
	TargetLogsGet     = 100 * time.Millisecond
)

// Valeurs par défaut des limites de ressources (cgroups).
const (
	// DefaultMemoryLimitMB est la limite mémoire par défaut d'un conteneur.
	DefaultMemoryLimitMB = 512
	// DefaultCPUQuota est le quota CPU par défaut en pourcentage (100 = 1 cœur).
	DefaultCPUQuota = 100
	// DefaultPidsLimit borne le nombre de processus dans un conteneur.
	DefaultPidsLimit = 256
)

// DefaultLogLines est le nombre de lignes de journal affichées par défaut
// (mode non-suivi). Voir pkg/logs.
const DefaultLogLines = 100

// Paramètres SSH (voir rules/security.md → SSH Hardening).
const (
	// SSHPortMin est le port SSH minimal (> 10000 pour éviter les conflits).
	SSHPortMin = 10001
	// SSHPortMax borne la plage de ports SSH allouables.
	SSHPortMax = 60000
	// SSHKeyType impose le type de clé (ED25519 minimum).
	SSHKeyType = "ed25519"
	// SSHKeyPerm est la permission stricte exigée sur les clés privées.
	SSHKeyPerm = 0o600
	// SSHUser est l'utilisateur cible par défaut à l'intérieur du conteneur.
	SSHUser = "root"
	// SSHHost est l'hôte d'accès : le port SSH est redirigé sur la loopback de
	// l'hôte (voir --port dans pkg/nspawn) et n'est jamais exposé publiquement.
	SSHHost = "127.0.0.1"
)

// SSHKeysDir contient les paires de clés gérées par l'application (une par
// conteneur). Sous StateDir pour rester avec le reste de l'état applicatif.
const SSHKeysDir = StateDir + "/keys"

// Paramètres du reverse proxy nginx (voir rules/security.md → Nginx Validation).
const (
	// NginxRateLimit est le taux de requêtes autorisé par IP (rate limiting).
	NginxRateLimit = "10r/s"
	// NginxRateBurst est la rafale tolérée au-delà du taux nominal.
	NginxRateBurst = 20
	// NginxMaxBodySize borne la taille des requêtes (client_max_body_size).
	NginxMaxBodySize = "10m"
)

// Drapeaux de sécurité systemd-nspawn imposés à chaque conteneur
// (voir rules/security.md → Isolation Enforcement).
//
// Ces drapeaux constituent le socle "defense in depth" et ne doivent
// jamais être désactivés sans revue de sécurité explicite.
var SecurityFlags = []string{
	"--read-only", // rootfs en lecture seule
	"--tmpfs=/tmp:noexec,nodev,nosuid",
	"--tmpfs=/var/tmp:noexec,nodev,nosuid",
}
