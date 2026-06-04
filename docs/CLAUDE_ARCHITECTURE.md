# Architecture Claude Code - Container Isolation Manager

## 📐 STRUCTURE HIÉRARCHIQUE DU PROJET

```
isolation-manager/
├── .claude/                           # Configuration Claude Code
│   ├── rules/                         # Règles d'architecture
│   ├── skills/                        # Skills personnalisées
│   ├── agents/                        # Subagents
│   ├── mcps/                          # MCPs intégrés
│   └── CLAUDE.md                      # Configuration maître
│
├── src/
│   ├── cmd/                           # Points d'entrée CLI
│   │   └── main.go                    # Entrée (build: ./src/cmd)
│   ├── pkg/                           # Packages métier
│   │   ├── nspawn/
│   │   ├── container/
│   │   ├── cgroups/
│   │   ├── ssh/
│   │   ├── nginx/
│   │   └── logs/
│   ├── internal/                      # Code interne
│   └── config/                        # Configurations
│
├── tests/                             # Tests
├── docs/                              # Documentation
├── go.mod                             # Dépendances
└── Makefile                           # Build
```

---

## 🎯 SKILLS PERSONNALISÉES À CRÉER

### 1. **SKILL: container-architecture**
**Objectif**: Valider l'architecture du code conteneurisation
```
Triggers: any file in pkg/*/
Validations:
  - Interface Containerizer implémentée
  - Tous les errors gérés
  - Logging cohérent
  - Tests unitaires présents
```

### 2. **SKILL: golang-standards**
**Objectif**: Standards Go (performance, sécurité)
```
Triggers: *.go files
Checks:
  - Pas de goroutine leak
  - Context timeout sur opérations bloquantes
  - Error wrapping avec fmt.Errorf("%w", err)
  - Pas de panic() en production
  - Race detector: go run -race
```

### 3. **SKILL: security-review**
**Objectif**: Sécurité isolation conteneur
```
Triggers: pkg/nspawn/*, pkg/ssh/*
Reviews:
  - Bind mounts correctement limités
  - Permissions fichiers restrictives (0700)
  - SSH key validation
  - Pas d'escalade privilège
  - seccomp filters appliqués
```

### 4. **SKILL: cli-ux**
**Objectif**: Interface utilisateur cohérente
```
Triggers: cmd/*, frontend components
Standards:
  - Bubble Tea patterns uniformes
  - Error messages clairs
  - Progress indicators
  - Color coding cohérent
```

### 5. **SKILL: nginx-parser**
**Objectif**: Validation parseur nginx
```
Triggers: pkg/nginx/*
Validations:
  - Parser handles tous cas nginx
  - Config validation complète
  - Tests contre configs réelles
  - Performance O(n)
```

---

## 🤖 SUBAGENTS À CONFIGURER

### Agent 1: **NSPAWN_EXPERT**
**Responsabilité**: Conteneurisation systèmd-nspawn
```yaml
scope:
  - Tout code pkg/nspawn/
  - Configuration systemd
  
skills:
  - container-architecture
  - security-review
  - golang-standards

rules:
  - Pas de hardcoding paths
  - Toujours utiliser exec.Command
  - Logging de tous appels système
  - Timeout sur exec.Cmd

decisions:
  - Valide tous les changements nspawn
  - Approuve uniquement sécurisé
```

### Agent 2: **CLI_BUILDER**
**Responsabilité**: Interface ligne de commande
```yaml
scope:
  - cmd/
  - TUI components

skills:
  - cli-ux
  - golang-standards

rules:
  - Utiliser Cobra pour CLI
  - Bubble Tea pour TUI
  - Responsive design
  - Help text complet

decisions:
  - Valide UX/DX
  - Approuve navigation
```

### Agent 3: **NETWORKING_EXPERT**
**Responsabilité**: SSH, Nginx, Proxy
```yaml
scope:
  - pkg/ssh/
  - pkg/nginx/
  - proxy logic

skills:
  - security-review
  - golang-standards

rules:
  - SSH: key-only auth
  - TLS for proxy
  - Rate limiting
  - Timeout gestion

decisions:
  - Valide sécurité réseau
  - Port binding validation
```

### Agent 4: **MONITORING_AGENT**
**Responsabilité**: Logs, Metrics, Cgroups
```yaml
scope:
  - pkg/logs/
  - pkg/cgroups/
  - metrics collectors

skills:
  - golang-standards
  - container-architecture

rules:
  - Pas de blocking I/O
  - Goroutines goroutine pools
  - Memory-efficient parsing
  - Real-time streaming

decisions:
  - Performance review
  - Scalability check
```

---

## 📋 RULES D'ARCHITECTURE GLOBALES

### Architecture Layer Rules
```
.claude/rules/architecture.md:

1. LAYERING (Separation of Concerns)
   ✓ cmd/     → CLI handlers only
   ✓ pkg/     → Business logic + interfaces
   ✓ internal/ → Private utilities
   ✓ config/  → Constants & defaults

2. DEPENDENCY FLOW
   ✓ cmd → pkg (one direction)
   ✓ pkg → std library, third-party
   ✓ NO circular dependencies
   ✓ Interface-based abstractions

3. ERROR HANDLING
   ✓ Custom errors in pkg/errors/
   ✓ Always wrap with context
   ✓ Log before returning
   ✓ Structured logging

4. CONCURRENCY RULES
   ✓ Use context.Context everywhere
   ✓ Goroutine pools for I/O
   ✓ WaitGroup for sync
   ✓ No unbounded goroutines

5. TESTING RULES
   ✓ >80% coverage pkg/
   ✓ *_test.go files alongside
   ✓ Table-driven tests
   ✓ Mocks for external systems
```

### Security Rules
```
.claude/rules/security.md:

1. ISOLATION ENFORCEMENT
   ✓ No host filesystem access without bind
   ✓ Containers have read-only root
   ✓ tmpfs for /tmp, /var/tmp
   ✓ No privileged containers

2. SSH HARDENING
   ✓ Key-based only (no passwords)
   ✓ ED25519 keys minimum
   ✓ SSH agent forwarding disabled
   ✓ Port > 10000 (avoid conflicts)

3. NGINX VALIDATION
   ✓ No upstream to host
   ✓ TLS enforced
   ✓ Rate limiting on proxy
   ✓ Request size limits

4. CODE SECURITY
   ✓ No hardcoded secrets
   ✓ Input validation everywhere
   ✓ Output escaping (template/html)
   ✓ No unsafe pointers
```

---

## 🔌 MCPs À INTÉGRER

### MCP 1: **filesystem-access**
```yaml
usage: 
  - Créer/modifier fichiers config
  - Read rootfs templates
  
triggered_by: Container creation
```

### MCP 2: **shell-execution** 
```yaml
usage:
  - Exécuter systemd-nspawn
  - journalctl queries
  - systemctl commands
  
constraints:
  - Only approved systemd commands
  - Timeout 30s par défaut
```

### MCP 3: **web-server** (optionnel)
```yaml
usage:
  - Dashboard web pour monitoring
  - Visualiser containers
  - Real-time logs

triggered_by: /dashboard command
```

---

## 📊 SKILL FLOW DIAGRAM

```
User Request
    ↓
[CLI_BUILDER Agent]
    ↓ parses command
    ↓
[Appropriate Expert Agent]
    ├→ [NSPAWN_EXPERT]     (create/delete)
    ├→ [NETWORKING_EXPERT] (ssh/nginx)
    ├→ [MONITORING_AGENT]  (logs/stats)
    ↓
[Security Review Skill]  ← Applied by all agents
    ↓
[Go Standards Skill]     ← Applied by all agents
    ↓
Execute with MCPs
    ↓
Result returned
```

---

## 🚀 CLAUDE.md MASTER CONFIG

```yaml
# .claude/CLAUDE.md

name: "Container Isolation Manager"
version: "1.0.0"
language: "Go 1.22+"

description: |
  Système d'isolation utilisateur avec conteneurs systemd-nspawn.
  Gestion complète: création, SSH, logs, nginx proxy.

philosophy:
  - Performance: <5% overhead vs host
  - Security: Defense in depth
  - Simplicity: No unneeded layers
  - Auditability: Full logging

active_skills:
  - container-architecture
  - golang-standards
  - security-review
  - cli-ux
  - nginx-parser

active_agents:
  - nspawn_expert
  - cli_builder
  - networking_expert
  - monitoring_agent

integrated_mcps:
  - filesystem-access
  - shell-execution
  - web-server (optional)

code_standards:
  line_length: 100
  indent: tabs
  test_coverage: 80%
  complexity: cyclomatic < 10

commit_rules:
  - Each commit must pass tests
  - No secrets in commits
  - Security review before merge
  - Changelog updated
```

---

## 📝 SKILL FILE STRUCTURE

Each skill should be in `.claude/skills/`:

```
skills/
├── container-architecture.md
├── golang-standards.md
├── security-review.md
├── cli-ux.md
└── nginx-parser.md
```

Example format:
```markdown
# Skill: Container Architecture

## Trigger Patterns
- `pkg/**/` (any package code)
- `**/container*.go`

## Validation Rules
1. Interface compliance
   - Check Containerizer interface implemented
   - All methods have proper error handling
   
2. Code structure
   - Separate concerns
   - No cyclic imports
   
## Review Checklist
- [ ] Interface defined
- [ ] Errors wrapped properly
- [ ] Tests written
- [ ] Logging added
- [ ] Comments for exported items

## Auto-fixes Available
- gofmt applied
- goimports resolved
```

---

## 🔄 AGENT COORDINATION EXAMPLE

**Scenario**: Ajout nouvelle feature SSH

1. **User**: "Add SSH key rotation"
2. **CLI_BUILDER** evaluates command structure
3. **NETWORKING_EXPERT** reviews SSH logic
4. **security-review skill** validates keys
5. **golang-standards skill** checks code
6. **Run tests** via shell-execution MCP
7. **Approve** or suggest fixes

---

## ⚡ QUICK START WORKFLOW

```bash
# 1. Initialize project structure
claude code init --template go-cli

# 2. Load skills
claude code skill add .claude/skills/container-architecture.md

# 3. Configure agents
claude code agent add .claude/agents/nspawn-expert.yaml

# 4. Enable MCPs
claude code mcp add shell-execution filesystem-access

# 5. Run with full context
claude code dev --agent nspawn_expert

# 6. Review with all skills
claude code review --comprehensive
```

---

## 📈 SCALABILITY CHECKLIST

- [ ] Skills are reusable across files
- [ ] Agents have clear non-overlapping domains
- [ ] Rules are documented and automated
- [ ] MCPs are composable
- [ ] Tests validate all agents
- [ ] Performance budgets defined
- [ ] Security gates implemented
