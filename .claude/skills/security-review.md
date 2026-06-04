# Skill: Security Review

## Purpose
Valider l'efficacité de l'isolation des containers et la sécurité réseau.
Voir aussi `rules/security.md`.

## Trigger Patterns
- pkg/nspawn/**/*.go
- pkg/ssh/**/*.go
- pkg/nginx/**/*.go

## Validation Rules

### Isolation container
- [ ] Aucun flag `--privileged`
- [ ] Root filesystem en lecture seule (`--read-only`)
- [ ] tmpfs pour `/tmp` et `/var/tmp` (noexec, nodev, nosuid)
- [ ] Bind mounts whitelistés uniquement (pas de wildcard)
- [ ] Aucun accès host `/dev`
- [ ] Limites cgroup appliquées
- [ ] Profil seccomp ou apparmor

```go
// ✅ Structure REQUISE
nspawnCmd := []string{
    "--machine=" + cfg.Name,
    "--directory=" + cfg.Rootfs,
    "--read-only",
    "--tmpfs=/tmp:noexec,nodev,nosuid",
    "--tmpfs=/var/tmp:noexec,nodev",
    "--bind-ro=/usr/bin:" + cfg.RootfsPath + "/usr/bin",
}
// ❌ PAS de --privileged, PAS de --cap-add, PAS de bind wildcard
```

### SSH Hardening
- [ ] Authentification par clé uniquement (pas de mot de passe)
- [ ] Clés ED25519 minimum, permissions 0600
- [ ] Agent forwarding désactivé
- [ ] Port > 10000

### Nginx
- [ ] Aucun upstream vers l'hôte
- [ ] TLS imposé
- [ ] Rate limiting + limites de taille des requêtes

### Code
- [ ] Aucun secret en dur
- [ ] Validation des entrées partout
- [ ] Échappement des sorties (html/template)
- [ ] Aucun `unsafe.Pointer`, pas d'escalade de privilège

## Rejections automatiques
```go
args = append(args, "--privileged")        // ❌ REJECTED
args = append(args, "--bind=/:" + rootfs)   // ❌ REJECTED (trop large)
cmd.Run() // sans timeout                    // ❌ REJECTED
_ = m.setupCgroups()                         // ❌ REJECTED (erreur ignorée)
```

## Security Gates (CI)
```bash
grep -r "\--privileged" src/   # FAIL si trouvé
grep -r "password" src/pkg/ssh # FAIL si trouvé
gitleaks protect --verbose     # FAIL si secrets
```
