# Rules: Architecture Globale

Règles d'architecture appliquées à tout le code du projet.
Source: `docs/CLAUDE_ARCHITECTURE.md` → "RULES D'ARCHITECTURE GLOBALES".

## 1. Layering (Separation of Concerns)
- `cmd/`      → CLI handlers uniquement
- `pkg/`      → Logique métier + interfaces
- `internal/` → Utilitaires privés
- `config/`   → Constantes & valeurs par défaut

## 2. Dependency Flow
- `cmd → pkg` (une seule direction)
- `pkg → std library, third-party`
- AUCUNE dépendance circulaire
- Abstractions basées sur des interfaces

## 3. Error Handling
- Erreurs custom dans `pkg/errors/`
- Toujours envelopper avec contexte: `fmt.Errorf("...: %w", err)`
- Logger avant de retourner
- Logging structuré (logrus)

## 4. Concurrency Rules
- Utiliser `context.Context` partout
- Goroutine pools pour les I/O
- `sync.WaitGroup` pour la synchronisation
- Aucune goroutine non bornée (no unbounded goroutines)

## 5. Testing Rules
- Coverage > 80% sur `pkg/`
- Fichiers `*_test.go` à côté du code
- Table-driven tests
- Mocks pour les systèmes externes (systemd-nspawn)

## Structure de projet attendue
```
src/
├── cmd/        # Points d'entrée CLI (main.go + sous-commandes Cobra)
│   └── main.go
├── pkg/        # Packages métier
│   ├── nspawn/
│   ├── container/
│   ├── cgroups/
│   ├── ssh/
│   ├── nginx/
│   └── logs/
├── internal/   # Code interne
└── config/     # Configurations & valeurs par défaut
```
Build: `go build -o bin/isolation-manager ./src/cmd`

## Review Checklist
- [ ] Respecte le layering cmd/pkg/internal/config
- [ ] Pas de dépendance circulaire
- [ ] Interfaces minimales (single responsibility)
- [ ] Erreurs enveloppées + loggées
- [ ] context.Context propagé
- [ ] Tests présents (>80%)
