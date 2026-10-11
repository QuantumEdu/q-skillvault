# Feature: Suite vault_mem_* y Emancipación de Memoria en SkillVault

- **Feature:** `vault-mem-suite`
- **Objective:** Implementar en SkillVault la suite de memoria episódica `vault_mem_*` (`vault_mem_save`, `vault_mem_search`, `vault_mem_context`, `vault_mem_get`, `vault_mem_timeline`) con deduplicación por hash normalizado y modo de compatibilidad opcional `--compat-engram`, integrando lo mejor de Engram sin colisión de nombres y permitiendo que `q-agent` opere con SkillVault como backend de memoria.
- **Why:** Permitir que `q-agent` y otros agentes utilicen SkillVault para persistencia continua de observaciones y contexto sin depender de Engram y sin colisionar con su servidor MCP si ambos coexisten.
- **Scope:**
  - Lógica de deduplicación normalizada y mapeo de tipos de observación en `internal/app/`.
  - Definición y registro de herramientas MCP `vault_mem_*` en `internal/mcp/tools.go`.
  - Soporte para flag `--compat-engram` en `skillvault mcp`.
  - Handler de `vault_mem_timeline` para contexto cronológico.
  - Suite de tests unitarios y de integración MCP para `vault_mem_*`.
  - Actualización de documentación y referencias en `q-agent`.
- **Constraints:**
  - Mantener Go puro (CGO_ENABLED=0, `modernc.org/sqlite`).
  - No romper ninguna herramienta MCP ni comando existente de SkillVault.
  - Cero colisiones de nombres con Engram cuando ambos servidores estén activos.
- **Effective TDD Mode:** disabled (ordinary functional checks and unit tests).

## Tasks

- [x] **TASK-01: Lógica de Deduplicación y Servicio de Memoria Episódica**
  - Implementar cálculo de hash normalizado (`sha256`) y detección de duplicados en `internal/app/`.
  - Mapear tipos de observación canónicos (`decision`, `architecture`, `bugfix`, `pattern`, `discovery`, `config`, `workflow`).
  - *Route:* delegated direct | *Checks:* tests unitarios en `internal/app/` PASS (6/6 tests).

- [x] **TASK-02: Definición y Registro de MCP Tools `vault_mem_*`**
  - Registrar esquemas MCP en `internal/mcp/tools.go` para `vault_mem_save`, `vault_mem_search`, `vault_mem_context`, `vault_mem_get`, `vault_mem_timeline`.
  - Implementar handlers correspondientes en `internal/mcp/tools.go`.
  - *Route:* delegated direct | *Checks:* tests unitarios de herramientas en `internal/mcp/` PASS.

- [x] **TASK-03: Modo de Compatibilidad `--compat-engram` y CLI**
  - Agregar soporte para `--compat-engram` en `internal/cli/mcp.go` y `internal/mcp/server.go`.
  - Cuando esté activo, registrar también los alias directos `mem_save`, `mem_search`, `mem_context`, `mem_get_observation`, `mem_timeline`.
  - *Route:* delegated direct | *Checks:* `TestEngramCompatModeMCP` PASS (44 tools registered).

- [x] **TASK-04: Verificación Integral de Suite de Tests**
  - Ejecutar `go test ./...` en `q-skillvault` y verificar 100% de tests en verde.
  - Recompilar e instalar binario actualizado en `~/.local/bin/skillvault`.
  - *Route:* direct inline | *Checks:* `go test ./...` 100% PASS, binario instalado y verificado con `echo '{"method":"tools/list"}' | skillvault mcp`.

- [x] **TASK-05: Documentación y Configuración en `q-agent`**
  - Actualizar `dual-engine.md` en `q-agent` documentando el uso de `vault_mem_*`.
  - *Route:* direct inline | *Checks:* commit `c311231` en `q-agent`.

