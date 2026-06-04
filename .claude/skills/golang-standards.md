# Skill: Go Standards

## Purpose
Imposer les standards Go de performance et de sécurité sur tout le code.

## Trigger Patterns
- Tout fichier `*.go`

## Validation Rules
1. **Pas de goroutine leak**
   - Toujours utiliser `context.Context`
   - Annuler les goroutines sur `ctx.Done()`
   ```go
   go func(ctx context.Context) {
       for {
           select {
           case <-ctx.Done():
               return
           default:
               readLogs()
           }
       }
   }(ctx)
   ```

2. **Pas de race condition**
   - Vérifier avec `go test -race ./...`

3. **Pas de blocage sans timeout**
   - `exec.CommandContext` requis (jamais `exec.Command` sur opération bloquante)
   - Timeout par défaut: 30s
   ```go
   ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
   defer cancel()
   cmd := exec.CommandContext(ctx, "systemd-nspawn", ...)
   ```

4. **Error wrapping**
   - `fmt.Errorf("context: %w", err)`
   - Pas de `panic()` en production (sauf `init()`)

5. **Performance**
   - Complexité cyclomatique < 10
   - Pas d'allocations dans les hot paths
   - Memory-efficient parsing

## Review Checklist
- [ ] context.Context propagé partout
- [ ] `go test -race` passe
- [ ] exec.CommandContext sur opérations système
- [ ] Erreurs enveloppées avec `%w`
- [ ] Pas de panic() runtime
- [ ] Complexité < 10

## Tooling
```bash
go vet ./...
gofmt -l . && goimports -w .
go test -race -cover ./...
golangci-lint run ./...
```
