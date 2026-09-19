---
name: q:skillvault
description: "Trigger: /q-skillvault, /skillvault, skillvault, q-skillvault, vault de skills, knowledge vault, base de conocimiento, line factory, mcp skillvault, context bundle. Knowledge Operating System local-first para desarrolladores y agentes IA: almacenamiento híbrido (SQLite+FTS5), compilación contextual de 7 modos, MCP stdio server, router semántico y fábrica determinista issue-to-PR (line)."
version: 3.1.0
license: MIT
metadata:
  author: Gabriel Magallon Sanchez - Qu@antum
  date: "2026-09-19"
  updated_at: "2026-09-19"
  repository: "https://github.com/QuantumEdu/q-skillvault"
---

# q:skillvault · Qu@ntum Knowledge OS

## Objetivo y Filosofía

**q-skillvault** (Qu@ntum) es un sistema operativo de conocimiento local-first diseñado específicamente para cerrar la brecha entre desarrolladores de software y agentes de Inteligencia Artificial.

- **Autor:** Gabriel Magallon Sanchez - Qu@antum
- **Organización:** QuantumEdu
- **Licencia:** MIT ([LICENSE](https://github.com/QuantumEdu/q-skillvault/blob/main/LICENSE))
- **Repositorio:** [https://github.com/QuantumEdu/q-skillvault](https://github.com/QuantumEdu/q-skillvault)

### Principios Fundamentales
1. **Local-First & Zero Heavy Frameworks**: Binario único compilado en Go puro (`modernc.org/sqlite` sin CGO). Sin dependencias de servicios cloud obligatorios ni daemons pesados.
2. **Almacenamiento Híbrido Estricto**:
   - *Metadatos, taxonomía LifeOS y FTS5* residen en SQLite (`~/.skillvault/vault.db`).
   - *Artefactos extensos* (análisis PDF, reportes, specs, logs) se persisten en disco (`~/.skillvault/objects/`) para no saturar la ventana de contexto de los modelos.
3. **El Contexto como Producto (Qu@ntum Context Layer)**: No basta con almacenar; entrega la cantidad justa de contexto mediante 7 modos especializados, jerarquía de prioridades y truncado adaptativo consciente del presupuesto de tokens.
4. **Fábrica Determinista Issue-to-PR (`line`)**: Orquestación agéntica de ciclo completo (plan → build → review → wait_ci → ready) ejecutada en git worktrees aislados con evaluador independiente.

---

## Cuándo Activar este Skill

Activar inmediatamente cuando:
- El usuario ejecute `/q-skillvault`, `/skillvault`, `line`, o mencione el vault de skills o fábrica de PRs.
- Se requiera persistir, consultar o indexar prompts, skills, decisiones de arquitectura o resúmenes de sesión.
- Un agente necesite inyectar contexto curado de un proyecto o flujo (`skillvault context --mode ...`).
- Se desee automatizar la resolución de un GitHub Issue hasta generar un Pull Request probado y verificado mediante `line run`.
- Se administren múltiples proyectos locales en la fábrica de código (`line project`).
- Se requiera configurar el servidor MCP de SkillVault para Claude Code, OpenCode o Antigravity.

---

## Ecosistema de Componentes

| Componente | Tipo | Rol Principal |
|------------|------|---------------|
| `skillvault` | Binario CLI / MCP | Vault central de conocimiento, taxonomía LifeOS, compilador de contexto y servidor MCP. |
| `line` | Binario CLI / TUI | Fábrica issue-to-PR determinista, gestión de proyectos y worktrees, TUI interactivo Bubble Tea. |
| `telemetryd` | Daemon Unix | Daemon local para telemetría agéntica, captura de señales y métricas de ejecución. |
| `telemetryctl` | Binario CLI | Consultas de telemetría, inspección de runs y estado de salud del daemon. |
| `q-secrets` | Submódulo / CLI | Gestor de secretos locales cifrados con integración keyring/master-key. |

---

## Referencia Rápida de Comandos

### 1. Gestión del Vault (`skillvault`)
```bash
# Inicialización y verificación
skillvault init                     # Crea ~/.skillvault/ (DB + directorios)
skillvault init --with-secrets      # Inicializa vault y prepara gestor de secretos
skillvault version                  # Versión y build metadata

# Guardar y consultar conocimiento
skillvault save "Auth JWT" --type skill --content "Reglas para tokens JWT..."
skillvault save "Decision SQLite" --type decision --project "pos-app" --content "Usar FTS5..."
skillvault search "JWT"             # Búsqueda semántica FTS5
skillvault list --type skill        # Listar entradas por tipo
skillvault get <id-o-slug>          # Ver detalle de una entrada

# Compilación de contexto agéntico (Qu@ntum Layer)
skillvault context --mode project --name "pos-app" --max-chars 8000
skillvault context --mode planning --project "pos-app"
skillvault context --mode full_brief --project "pos-app"

# Ruteo semántico de intención
skillvault route "necesito auditar la seguridad de las APIs"
```

### 2. Fábrica Determinista Issue-to-PR (`line`)
```bash
# Gestión multi-proyecto
line project add pos-core --name "POS Engine" --repo /ruta/al/repo --exec claude --ntfy-topic pos-alerts
line project list                   # Listar proyectos (* indica el activo)
line project use pos-core           # Cambiar proyecto activo
line project current                # Ver proyecto activo actual

# Ejecución de fábrica (usa el repo activo si no se pasa --repo)
line run https://github.com/QuantumEdu/pos-app/issues/101
line run --auto=false https://github.com/QuantumEdu/pos-app/issues/101  # Modo manual paso a paso

# Control de ejecución y desbloqueo humano
line status                         # Estado del job actual
line continue --answer "Usar tasa de IVA 16%"  # Responder a needs_human
line next                           # Avanzar de fase manualmente
line abort                          # Abortar job actual y limpiar worktree

# Interfaz interactiva de terminal (Bubble Tea)
line tui                            # Dashboard interactivo con tabla de jobs e inspector

# Servidor Web UI
line serve --port 7340              # Dashboard web local en 127.0.0.1:7340
```

---

## Integración MCP (Model Context Protocol)

Para exponer el conocimiento del vault a agentes (Claude Code, OpenCode, Cursor, Antigravity), configurar en el archivo de clientes MCP:

```json
{
  "mcpServers": {
    "skillvault": {
      "command": "skillvault",
      "args": ["mcp"]
    }
  }
}
```

### Herramientas MCP expuestas:
- `search(query, type, project)`: Búsqueda FTS5 en el vault.
- `get_context(mode, project, max_chars)`: Compila bundle de contexto.
- `save_entry(title, type, content, project, tags)`: Guarda nueva entrada.
- `save_artifact(title, type, content, project)`: Guarda artefacto largo en disco + metadatos.
- `save_result(run_id, summary, artifacts)`: Registra resultado de ejecución.
- `route_scenario(scenario)`: Resuelve escenario a workflow o skill recomendado.
- `run_workflow(workflow_id, params)`: Ejecuta workflow multi-fase estructurado.

---

## Flujo Recomendado de Uso

1. **Al iniciar un proyecto**:
   - Registrar el repo en `line`: `line project add <id> --repo <path>`.
   - Inicializar el contexto del proyecto en `skillvault`: `skillvault save "Contexto Inicial" --type project ...`.
2. **Durante el desarrollo**:
   - Guardar decisiones técnicas y gotchas descubiertos con `skillvault save`.
   - Si surge un issue de GitHub, delegarlo a `line run <issue-url>` y monitorearlo con `line tui`.
   - En caso de preguntas agénticas (`needs_human`), responder desde el TUI con `[c]` o con `line continue --answer "..."`.
3. **Al cerrar sesión**:
   - Ejecutar `q:session-wrap` para sintetizar el progreso en Engram y catalogar notas clave en SkillVault.
