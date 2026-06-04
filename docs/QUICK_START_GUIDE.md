# 🚀 GUIDE DÉMARRAGE - Architecture Claude Code Complète

## 📋 RÉSUMÉ DE L'ARCHITECTURE

```
┌─────────────────────────────────────────────────────────────┐
│              ISOLATION MANAGER - Claude Code Setup           │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  SKILLS (5)                    AGENTS (4)                    │
│  ├─ container-architecture     ├─ NSPAWN_EXPERT             │
│  ├─ golang-standards           ├─ CLI_BUILDER               │
│  ├─ security-review            ├─ NETWORKING_EXPERT         │
│  ├─ cli-ux                     └─ MONITORING_AGENT          │
│  └─ nginx-parser               │                             │
│                                └─ Validation de qualité     │
│                                                               │
│  RULES (2)                     MCPs (3)                      │
│  ├─ architecture.md            ├─ filesystem-access         │
│  └─ security.md                ├─ shell-execution          │
│                                └─ web-server (opt)         │
│                                                               │
└─────────────────────────────────────────────────────────────┘

FLUX DE TRAVAIL:
User Input → CLI_BUILDER → Specialized Agent → Skills validation
         → Security/Performance checks → Execute with MCPs → Result
```

---

## 🎯 SETUP PAR ÉTAPE

### ÉTAPE 1: Initialiser la structure du projet

```bash
# Créer le repo
mkdir isolation-manager && cd isolation-manager
git init

# Structure Go standard
mkdir -p src/{cmd,pkg/{nspawn,container,cgroups,ssh,nginx,logs},internal,config}
mkdir -p tests docs .claude/{skills,agents,rules,mcps}

# Fichiers de base
touch go.mod go.sum Makefile main.go
touch .claude/CLAUDE.md

# Git setup
git add -A
git commit -m "Initial project structure"
```

### ÉTAPE 2: Créer les fichiers CLAUDE.md de configuration

**`.claude/CLAUDE.md`** (le fichier maître):
```yaml
name: "Container Isolation Manager"
version: "1.0.0"
language: "Go 1.22+"

philosophy:
  - Performance: < 5% overhead
  - Security: Defense in depth
  - Simplicity: No unneeded layers

# Charge automatiquement
active_skills:
  - ./skills/container-architecture.md
  - ./skills/golang-standards.md
  - ./skills/security-review.md
  - ./skills/cli-ux.md
  - ./skills/nginx-parser.md

active_agents:
  - ./agents/nspawn-expert.yaml
  - ./agents/cli-builder.yaml
  - ./agents/networking-expert.yaml
  - ./agents/monitoring-agent.yaml

integrated_mcps:
  - filesystem-access
  - shell-execution
  - web-server

code_standards:
  test_coverage: 80%
  cyclomatic_complexity: 10
  
security_gates:
  - no --privileged
  - no unvalidated mounts
  - no password auth SSH
  - all operations logged
```

### ÉTAPE 3: Skills dans `.claude/skills/`

Toutes les skills sont en place:
```
.claude/skills/
├── container-architecture.md     ✅
├── golang-standards.md           ✅
├── security-review.md            ✅
├── cli-ux.md                     ✅
└── nginx-parser.md               ✅
```

### ÉTAPE 4: Agents dans `.claude/agents/`

Les 4 agents sont configurés en `.yaml` (nspawn-expert, cli-builder,
networking-expert, monitoring-agent).

Structure YAML pour agents:
```yaml
# .claude/agents/nspawn-expert.yaml
name: NSPAWN_EXPERT
role: Container Isolation Manager
authority: HIGH

scope:
  owns:
    - pkg/nspawn/**/*.go
    - pkg/container/manager.go
    - pkg/cgroups/**/*.go
  
  reviews:
    - cmd/create.go
    - pkg/ssh/ (for container setup)

skills:
  - container-architecture
  - golang-standards
  - security-review

decisions:
  approve_if:
    - implements Containerizer
    - has security isolation
    - tests passing
    - proper logging
  
  reject_if:
    - has --privileged
    - unvalidated mounts
    - no error handling
    - blocking without timeout

performance_targets:
  create: 500ms
  start: 200ms
  stop: 100ms
```

### ÉTAPE 5: Créer les autres Skills manquants

**`golang-standards.md`** (extrait):
```markdown
# Skill: Go Standards

## Triggers
- Any .go file

## Validations
1. No goroutine leaks
   - Always use context.Context
   - Cancel goroutines on context.Done()

2. No race conditions
   - Use -race flag: go run -race

3. No blocking without timeout
   - exec.CommandContext required
   - Default timeout 30s

4. Error wrapping
   - Use fmt.Errorf("%w", err)

5. Performance
   - Complexity < 10 (cyclomatic)
   - No allocations in hot paths
```

### ÉTAPE 6: Configuration initiale Makefile

```makefile
# Makefile
.PHONY: help build test lint fmt race coverage

help:
	@echo "Available targets:"
	@echo "  make build      - Compile binary"
	@echo "  make test       - Run tests"
	@echo "  make race       - Check for race conditions"
	@echo "  make coverage   - Generate coverage report"
	@echo "  make lint       - Run linters"
	@echo "  make fmt        - Format code"

build:
	go build -o bin/isolation-manager ./src/cmd

test:
	go test -v ./...

race:
	go test -race ./...

coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...
	goimports -w .
```

---

## 🔄 WORKFLOW DÉVELOPPEMENT QUOTIDIEN

### Scénario: Ajouter une nouvelle feature

```
1. User demande: "Create container with memory limit"

2. CLI_BUILDER agent:
   ✓ Parse la commande
   ✓ Valide les paramètres
   ✓ Route vers NSPAWN_EXPERT

3. NSPAWN_EXPERT:
   ✓ Vérifie interface Containerizer
   ✓ Valide isolation
   ✓ Vérifie timeouts
   
4. Skills applicables:
   ✓ container-architecture: Structure OK?
   ✓ golang-standards: Pas de race condition?
   ✓ security-review: Isolation complète?
   
5. Éxécution:
   ✓ Execute systemd-nspawn (via shell-execution MCP)
   ✓ Setup cgroups (filesystem-access MCP)
   ✓ Configure logs (shell-execution MCP)
   
6. Résultat:
   ✓ Retour au user via CLI_BUILDER
```

### Commandes Claude Code Typiques

```bash
# 1. Démarrer session avec contexte complet
claude code dev --agent nspawn_expert

# 2. Créer nouvelle fonction
# (Claude applique automatiquement les skills)
claude code create pkg/nspawn/memory.go

# 3. Tester avec validation automatique
claude code test --coverage 80%

# 4. Revoir avec tous les agents
claude code review --comprehensive

# 5. Exécuter avec monitoring
claude code run --monitor-performance
```

---

## 📊 MATRICE DE RESPONSABILITÉ (RACI)

| Task | NSPAWN | CLI_BUILDER | NETWORKING | MONITORING |
|------|--------|------------|-----------|-----------|
| Create container | **R** | A | I | C |
| SSH setup | I | A | **R** | C |
| Nginx proxy config | I | A | **R** | C |
| Log collection | I | A | I | **R** |
| Performance tuning | **R** | C | A | I |
| Security validation | **R** | I | A | I |
| User interface | C | **R** | I | A |

Legend: R=Responsible, A=Accountable, C=Consulted, I=Informed

---

## 🛡️ SECURITY GATES APPLIQUÉS

Chaque commit doit passer:

```bash
# 1. No --privileged containers
grep -r "\--privileged" src/ → FAIL if found

# 2. All error paths have logging
ast-check "error" "log" → FAIL if mismatch

# 3. All systemd-nspawn calls have timeout
check "exec.CommandContext" "systemd-nspawn" → FAIL if missing

# 4. SSH uses key auth only
grep -r "password" src/pkg/ssh → FAIL if found

# 5. Test coverage >= 80%
go test -cover → FAIL if < 80%

# 6. No secrets in code
gitleaks protect --verbose → FAIL if secrets found

# 7. No race conditions
go test -race ./... → FAIL if races detected
```

---

## 📈 PROGRESSION DU PROJET

### Phase 1: Foundation (Week 1)
- [ ] Project structure setup
- [ ] CLAUDE.md configuration
- [ ] Core interfaces (Containerizer)
- [ ] Basic NSPAWN_EXPERT
- [ ] First container create/delete

### Phase 2: Core Features (Week 2)
- [ ] Container lifecycle (start/stop)
- [ ] Resource limits (cgroups)
- [ ] Logging integration (journalctl)
- [ ] CLI with Cobra
- [ ] CLI_BUILDER complete

### Phase 3: Advanced Features (Week 3)
- [ ] SSH integration
- [ ] Nginx parser
- [ ] Reverse proxy setup
- [ ] NETWORKING_EXPERT complete
- [ ] MONITORING_AGENT dashboard

### Phase 4: Polish (Week 4)
- [ ] Performance optimization
- [ ] Security hardening
- [ ] Full test coverage
- [ ] Documentation
- [ ] Production deployment

---

## 🧪 TESTING STRATEGY

### Unit Tests (per agent scope)
```
pkg/nspawn/ → 100% coverage (NSPAWN_EXPERT ensures)
pkg/ssh/ → 100% coverage (NETWORKING_EXPERT ensures)
pkg/logs/ → 100% coverage (MONITORING_AGENT ensures)
cmd/ → 80% coverage (CLI_BUILDER ensures)
```

### Integration Tests
```
tests/integration/
├── test_create_and_start.go
├── test_resource_limits.go
├── test_ssh_connection.go
└── test_nginx_config.go
```

### Security Tests
```
tests/security/
├── test_no_privilege_escape.go
├── test_mount_isolation.go
├── test_network_isolation.go
└── test_filesystem_isolation.go
```

---

## 🎓 EXEMPLE: Ajouter Feature SSH Key Rotation

```
Step 1: User demande
    "Add SSH key rotation for container user-01"

Step 2: CLI_BUILDER Agent
    - Parse: container=user-01, action=rotate-ssh-key
    - Route to NETWORKING_EXPERT

Step 3: NETWORKING_EXPERT
    - Valide SSH key format (ED25519)
    - Valide permissions (0600)
    - Valide no password auth remains
    - Approuve si tout OK

Step 4: Skills appliquées
    ✓ golang-standards: pas de blocking
    ✓ security-review: no password fallback
    ✓ container-architecture: logging complet

Step 5: Exécution
    - Read container SSH config (filesystem-access MCP)
    - Generate new key (shell-execution MCP)
    - Setup journalctl logging (shell-execution MCP)
    - Verify access works (shell-execution MCP)

Step 6: Retour user
    "✅ SSH keys rotated for user-01
     Old keys backed up to: /backup/user-01.keys.old
     Test connection: ssh user-01@container
     View logs: journalctl -m user-01"
```

---

## 🔧 TROUBLESHOOTING

### Problem: Agent rejects my change
**Solution**: Check the agent's approval criteria in `.claude/agents/`

### Problem: Tests failing locally but Claude approves
**Solution**: Run `go test -race` locally before submitting

### Problem: Skill not triggering
**Solution**: Verify file path matches trigger pattern in skill YAML

### Problem: MCP call failing
**Solution**: Check MCP availability with `claude code mcp list`

---

## 📚 FICHIERS À CRÉER POUR DÉMARRER

```
Essentiels (maintenant):
✅ .claude/CLAUDE.md
✅ .claude/skills/container-architecture.md
✅ .claude/agents/nspawn-expert.yaml
✅ .claude/rules/architecture.md
✅ .claude/rules/security.md

À compléter:
⏳ .claude/skills/golang-standards.md
⏳ .claude/skills/security-review.md
⏳ .claude/skills/cli-ux.md
⏳ .claude/skills/nginx-parser.md
⏳ .claude/agents/cli-builder.yaml
⏳ .claude/agents/networking-expert.yaml
⏳ .claude/agents/monitoring-agent.yaml

Code initial:
⏳ src/cmd/main.go
⏳ src/pkg/container/interface.go
⏳ src/pkg/nspawn/executor.go
⏳ Makefile
⏳ go.mod
```

---

## ✅ VALIDATION CHECKLIST

Avant de commencer le code:

- [ ] `.claude/CLAUDE.md` créé et testé
- [ ] Tous les skills présents dans `.claude/skills/`
- [ ] Tous les agents présents dans `.claude/agents/`
- [ ] Règles dans `.claude/rules/`
- [ ] MCPs configurés
- [ ] Makefile prêt
- [ ] go.mod initialisé
- [ ] Git repo configuré
- [ ] README.md documenté

Tu es prêt! 🚀

---

## 💡 NEXT STEPS

1. **Create `.claude/CLAUDE.md`** from CLAUDE_ARCHITECTURE.md
2. **Create skill files** in `.claude/skills/`
3. **Create agent yamls** in `.claude/agents/`
4. **Create rule files** in `.claude/rules/`
5. **Start development** with `claude code dev`

Voulez-vous que je créé les fichiers yaml des autres agents ou les autres skills?
