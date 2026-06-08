name: "Container Isolation Manager"
version: "1.0.0"
language: "Go 1.22+"

description: |
  Système d'isolation utilisateur avec conteneurs systemd-nspawn.
  Gestion complète du cycle de vie: création, SSH, logs, nginx proxy,
  limites de ressources (cgroups) et monitoring temps réel.

philosophy:
  - Performance: < 5% overhead vs host
  - Security: Defense in depth
  - Simplicity: No unneeded layers
  - Auditability: Full logging of all operations

# Chargés automatiquement à chaque session
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

# Règles d'architecture globales appliquées à tout le code
rules:
  - ./rules/architecture.md
  - ./rules/security.md

integrated_mcps:
  - name: filesystem-access
    enabled: true
    config: ./mcps/filesystem-access.yaml
  - name: shell-execution
    enabled: true
    config: ./mcps/shell-execution.yaml
  - name: web-server
    enabled: false  # Optionnel - activer avec --enable-dashboard
    config: ./mcps/web-server.yaml

code_standards:
  line_length: 100
  indent: tabs
  test_coverage: 80%
  cyclomatic_complexity: 10

# Portes de sécurité - chaque commit doit les franchir
security_gates:
  - no --privileged containers
  - no unvalidated / wildcard mounts
  - SSH key-only by default (password only via explicit --password opt-in)
  - all system operations logged (audit trail)
  - no hardcoded secrets (gitleaks)
  - no race conditions (go test -race)
  - test coverage >= 80%

commit_rules:
  - Each commit must pass tests
  - No secrets in commits
  - Security review before merge
  - Changelog updated

# Cible de performance par opération (voir agents/nspawn-expert.yaml)
performance_targets:
  container_create: 500ms
  container_start: 200ms
  container_stop: 100ms   # SIGTERM, 5s pour SIGKILL
  resource_read: 50ms
  logs_retrieve: 100ms    # par 1000 lignes
