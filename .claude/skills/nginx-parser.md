# Skill: Nginx Parser

## Purpose
Valider le parseur de configuration nginx et les configs générées pour le
reverse proxy des containers.

## Trigger Patterns
- pkg/nginx/**/*.go

## Validation Rules
1. **Parser robuste**
   - Gère tous les cas de configuration nginx
   - Validation de config complète
   - Testé contre des configs réelles
   - Performance O(n)

2. **Sécurité de la config générée** (voir security-review)
   - Aucun upstream vers l'hôte
   - TLS imposé sur le proxy
   - Rate limiting présent
   - Limites de taille des requêtes

## Review Checklist
- [ ] Parser gère les blocs imbriqués (http/server/location)
- [ ] Erreurs de syntaxe remontées clairement
- [ ] Tests table-driven contre configs réelles
- [ ] Complexité O(n), pas de backtracking exponentiel
- [ ] Config générée: TLS + rate limit + pas d'upstream hôte
- [ ] Coverage >80%
