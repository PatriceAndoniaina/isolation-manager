# Skill: Container Architecture

## Purpose
Valider que toute modification au code de conteneurisation respecte les
principes architecturaux: isolation effective, gestion complète du cycle
de vie, interfaces bien définies, testabilité.

## Trigger Patterns
```
Files matching:
  - pkg/nspawn/**/*.go
  - pkg/container/**/*.go
  - pkg/cgroups/**/*.go
  - **/container*.go
  - *_test.go (liés aux fichiers ci-dessus)
Exclude:
  - vendor/
  - test fixtures
```

## Core Validation Rules

### 1. Interface Compliance
Toute opération container DOIT implémenter l'interface `Containerizer`:
```go
type Containerizer interface {
    Create(ctx context.Context, config ContainerConfig) error
    Start(ctx context.Context, name string) error
    Stop(ctx context.Context, name string, timeout time.Duration) error
    Delete(ctx context.Context, name string) error
    Inspect(ctx context.Context, name string) (ContainerInfo, error)
    Logs(ctx context.Context, name string, follow bool) (io.ReadCloser, error)
    Exec(ctx context.Context, name string, cmd []string) (int, error)
    List(ctx context.Context) ([]ContainerInfo, error)
}
```
- ✅ context.Context en premier paramètre
- ✅ error en dernière valeur de retour
- ✅ Timeouts imposés (pas d'attente infinie)
- ✅ Nettoyage des ressources garanti

### 2. Error Handling Pattern
- Pas de retour d'erreur nu — envelopper: `fmt.Errorf("...: %w", err)`
- Logger avant de retourner
- Pas de `panic()` hors `init()`
- Pas d'échec silencieux

### 3. Resource Lifecycle Management
```
Create → Configure → Start → Monitor → Stop → Delete
```
- Pas de process orphelin, pas de mount point pendant
- Cleanup sur erreur (defer)
- Opérations idempotentes

### 4. Logging Standards (logrus structuré)
```go
logger := log.WithFields(logrus.Fields{
    "container": containerName,
    "operation": "create",
})
logger.Info("starting container creation")
logger.WithError(err).Error("mount failed")
```
- Entrée/sortie loggée pour chaque fonction publique
- Nom du container dans tous les champs
- Debug pour les détails de syscall

### 5. Testing Requirements
- Table-driven tests, cas d'erreur couverts
- Mock des appels systemd-nspawn
- Vérification du cleanup, scénarios de timeout
- Coverage > 80%

### 6. Security Isolation (voir security-review)
- Pas de `--privileged`, root en read-only
- tmpfs `/tmp` (noexec, nodev, nosuid)
- Binds whitelistés uniquement, pas d'accès host `/dev`
- Limites cgroup + profil seccomp/apparmor

## Review Checklist
- [ ] Implémente/utilise Containerizer correctement
- [ ] context.Context en premier param, error en dernier
- [ ] Erreurs enveloppées + loggées, pas de panic()
- [ ] Cleanup sur échec garanti, opérations idempotentes
- [ ] Entrée/sortie loggée, nom container présent
- [ ] Coverage >80%, mocks systemd-nspawn, timeouts testés
- [ ] --read-only, tmpfs, binds whitelistés, cgroups

## Auto-fixes Available
- Error wrapping: `return err` → `return fmt.Errorf("op failed: %w", err)`
- Logging addition autour des erreurs
- `gofmt` / `goimports`

## Common Issues & Fixes
- **Goroutine leak**: ajouter `select { case <-ctx.Done(): return }`
- **Mount cleanup**: `defer unmount(...)` dans Stop()
- **Context timeout**: `exec.CommandContext` au lieu de `exec.Command`

## References
- systemd-nspawn man page
- Linux namespaces (man7)
- cgroups v2 (kernel docs)
