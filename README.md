# Container Isolation Manager

Gestionnaire d'isolation utilisateur par conteneurs **systemd-nspawn**.
Cycle de vie complet : création, SSH durci, journaux, reverse proxy nginx,
limites de ressources (cgroups v2) et déploiement distant.

## Prérequis

| Environnement | Besoin |
|---|---|
| Poste local (dev / pilotage) | Go 1.22+, `ssh`, `rsync` |
| Serveur cible | Linux avec **systemd**, `systemd-container` (fournit `systemd-nspawn`, `machinectl`), accès **root** ou **sudo** |

> Les commandes de cycle de vie (`create`, `start`, `stats`…) ne s'exécutent
> que sur un hôte Linux disposant de `systemd-nspawn`. Depuis macOS/Windows, on
> pilote un serveur Linux via la commande `remote` (voir plus bas).

## Build

```bash
make build                 # → bin/isolation-manager
# ou
go build -o bin/isolation-manager ./src/cmd
```

## Commandes

| Commande | Rôle |
|---|---|
| `create <name>` | Créer un conteneur isolé (`--memory`, `--cpu`, `--pids`) |
| `list` (`ls`) | Lister les conteneurs |
| `start` / `stop` | Démarrer / arrêter |
| `destroy` (`rm`) | Détruire (rootfs + métadonnées) |
| `ssh <name>` | Session SSH durcie (clé ED25519, `--user`) |
| `logs <name>` | Journaux journald (`--follow`, `--lines`, `--since`) |
| `stats <name>` | Métriques cgroup v2 (`--watch`, `--interval`) |
| `nginx <name>` | Générer la config reverse proxy durcie |
| `nginx fmt <fichier>` | Reformater un fichier nginx (`-w` pour écrire sur place) |
| `nginx validate <fichier>` | Valider syntaxe + règles de sécurité d'un fichier nginx |
| `nginx list <dossier>` | Lister les fichiers `*.conf` d'un dossier (`-c` pour valider chacun) |
| `deploy` | Déployer l'outil sur un serveur Linux distant |
| `remote` | Exécuter une commande sur le serveur distant |

Flags globaux : `-v/--verbose`, `--json`, `--version`, `--auto-install`.

---

## Preflight : vérification automatique des dépendances

Avant d'exécuter une commande, l'outil vérifie (`exec.LookPath`) que les
binaires externes qu'elle utilise réellement sont présents sur l'hôte **où il
s'exécute**. S'ils manquent, il les **installe automatiquement** via le
gestionnaire de paquets de la distribution.

| Commande | Binaires vérifiés |
|---|---|
| `start` | `systemd-run`, `systemd-nspawn` |
| `stop` | `machinectl` |
| `ssh` | `ssh`, `ssh-keygen` |
| `logs` | `journalctl` |
| `deploy` | `ssh`, `rsync`, `go` |
| `remote` | `ssh` |

- Détecte **apt / dnf / pacman** et installe le bon paquet
  (`systemd-nspawn` → `systemd-container`, `ssh` → `openssh-client`…).
- `--auto-install=false` : se contente de **signaler** les manquants sans rien
  installer (échec explicite).
- Le preflight s'exécute **là où tourne le binaire** : ainsi
  `remote --host srv start user01` vérifie et installe les dépendances
  **sur le serveur**, pas en local.

```bash
# sur un serveur fraîchement provisionné, start installe systemd-container au besoin
isolation-manager start user01
→ dépendances manquantes (apt) : systemd-nspawn — installation de systemd-container…
```

> ⚠️ L'auto-install passe par `sudo` en mode non-interactif : **sudo sans mot de
> passe** est requis. Sinon, utilise `deploy --install-deps` ou installe
> `systemd-container` à la main.

---

## Workflow distant (déploiement + pilotage)

Idée : on donne un **accès SSH** à un serveur Linux ; l'outil détecte son OS,
s'y transfère et y installe ses dépendances. Ensuite, on pilote les conteneurs
à distance — sans avoir à copier quoi que ce soit à la main.

### 1. Déployer l'outil sur le serveur — `deploy`

```bash
isolation-manager deploy --host srv.example.com --user admin -i ~/.ssh/id_ed25519
```

Étapes exécutées automatiquement :

1. **SSH** vers le serveur et détection OS/architecture (`uname -s -m`) — refuse
   tout ce qui n'est pas Linux.
2. **Compilation croisée locale** du binaire pour l'architecture détectée
   (`GOOS=linux GOARCH=amd64|arm64`).
3. **Transfert `rsync`** vers `/tmp`, puis installation via
   `sudo install -m 0755` à l'emplacement final.
4. **Dépendances** : installe `systemd-container` s'il est absent
   (détecte `apt`/`dnf`/`pacman`).

| Flag | Défaut | Description |
|---|---|---|
| `--host` | *(requis)* | hôte du serveur |
| `-u, --user` | `root` | utilisateur SSH |
| `-p, --port` | `22` | port SSH |
| `-i, --key` | — | clé privée SSH |
| `--remote-path` | `/usr/local/bin/isolation-manager` | emplacement d'installation |
| `--source` | `.` | racine du module à compiler |
| `--install-deps` | `true` | installer `systemd-container` si absent |

> ⚠️ `deploy` se lance **depuis le dépôt** (il compile `./src/cmd`). Utilise
> `--source` pour pointer ailleurs. Go est requis en local.

### 2. Piloter les conteneurs à distance — `remote`

```bash
isolation-manager remote --host srv.example.com create user01 --memory 1024
isolation-manager remote --host srv.example.com start  user01
isolation-manager remote --host srv.example.com stats  user01 --watch
isolation-manager remote --host srv.example.com logs   user01 --follow
isolation-manager remote --host srv.example.com destroy user01
```

`remote` ouvre une session SSH (avec pseudo-terminal pour l'interactif comme
`stats --watch` ou `ssh`) et exécute `sudo <binaire-distant> <commande>`.

> ⚠️ Les flags de `remote` doivent **précéder** la sous-commande : tout ce qui
> suit le premier argument est transmis tel quel au serveur.
> `remote --host srv create user01` ✅ — `remote create user01 --host srv` ❌

### Alternative : connexion manuelle

Une fois `deploy` effectué, le binaire est dans le `PATH` du serveur :

```bash
ssh admin@srv.example.com
isolation-manager create user01
isolation-manager list
```

### Prérequis du workflow distant

- **sudo sans mot de passe** sur le serveur (installation dans `/usr/local/bin`
  et de `systemd-container` en mode non-interactif).
- Confiance SSH en **TOFU** (`StrictHostKeyChecking=accept-new`).

---

## Sécurité

- Conteneurs en `--read-only` + tmpfs durci (`noexec,nodev,nosuid`), aucun
  `--privileged`/`--capability` (garde-fou à l'exécution).
- SSH par **clé uniquement** (ED25519), pas de mot de passe, agent forwarding
  désactivé, clés en `0600`.
- nginx généré : **TLS imposé**, rate limiting, limite de taille des requêtes,
  aucun upstream vers l'hôte (config revalidée après génération).
- Toutes les opérations système sont journalisées (audit trail).

## Développement

```bash
make test       # tests
make race       # tests -race
make coverage   # couverture (HTML) — mesure sur ./src/pkg/...
make lint       # golangci-lint
make vet        # go vet
```

Architecture en couches : `cmd/` (CLI) → `pkg/` (métier) → `internal/` /
`config/`. Voir `.claude/rules/` et `docs/`.
