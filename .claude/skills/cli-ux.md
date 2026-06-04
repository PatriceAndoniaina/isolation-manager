# Skill: CLI UX

## Purpose
Garantir une interface ligne de commande cohérente et une bonne UX/DX.

## Trigger Patterns
- cmd/**/*.go
- composants TUI

## Validation Rules
- **Framework**: Cobra pour la CLI, Bubble Tea pour le TUI
- **Patterns Bubble Tea uniformes** entre les écrans
- **Messages d'erreur clairs et actionnables** (suggérer une commande de
  récupération)
- **Progress indicators** sur les opérations longues
- **Color coding cohérent** (succès/erreur/info)
- **Help text complet** sur chaque commande et flag
- **Responsive design** du TUI

## Exemple de feedback
```
✅ Container user-01 created
Status: Running
Memory: 512MB limit applied
Logs: journalctl -m user-01
```
```
❌ Failed to start container: cgroup memory limit too low.
   Try: isolation delete user-01 && isolation create --memory 512M user-01
```

## Commandes CLI cibles
```bash
isolation create --memory 512M user-01
isolation delete user-01
isolation logs user-01 --follow
isolation ssh user-01
isolation config nginx user-01
```

## Review Checklist
- [ ] Commande structurée avec Cobra
- [ ] Help text + exemples présents
- [ ] Messages d'erreur clairs avec recovery
- [ ] Progress indicator si opération > 1s
- [ ] Color coding cohérent
