# Rules: Sécurité

Règles de sécurité appliquées à toute la conteneurisation et au réseau.
Source: `docs/CLAUDE_ARCHITECTURE.md` → "Security Rules".

## 1. Isolation Enforcement
- Aucun accès au filesystem hôte sans bind explicite
- Containers avec root en lecture seule (`--read-only`)
- tmpfs pour `/tmp` et `/var/tmp` (noexec, nodev, nosuid)
- Aucun container privilégié (`--privileged` interdit)
- Aucun accès host `/dev`
- Limites cgroup appliquées
- Profil seccomp ou AppArmor

## 2. SSH Hardening
- Authentification par clé uniquement (pas de mot de passe)
- Clés ED25519 minimum
- SSH agent forwarding désactivé
- Port > 10000 (éviter les conflits)
- Permissions clés restrictives (0600)

## 3. Nginx Validation
- Aucun upstream vers l'hôte
- TLS imposé
- Rate limiting sur le proxy
- Limites de taille des requêtes

## 4. Code Security
- Aucun secret en dur (hardcoded)
- Validation des entrées partout
- Échappement des sorties (`html/template`)
- Aucun `unsafe.Pointer`

## Security Gates (par commit)
```bash
grep -r "\--privileged" src/            # FAIL si trouvé
grep -r "password" src/pkg/ssh          # FAIL si trouvé
check exec.CommandContext systemd-nspawn # FAIL si timeout absent
go test -cover ./...                     # FAIL si < 80%
gitleaks protect --verbose               # FAIL si secrets
go test -race ./...                      # FAIL si race
```

## Review Checklist
- [ ] `--read-only` + tmpfs configurés
- [ ] Aucun `--privileged` / `--cap-add`
- [ ] Binds whitelistés uniquement
- [ ] Limites cgroup posées
- [ ] SSH key-only, ED25519, port > 10000
- [ ] Nginx: TLS + rate limit + pas d'upstream hôte
- [ ] Pas de secret en dur, entrées validées
