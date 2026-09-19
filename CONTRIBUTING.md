# Contributing to q-skillvault

¡Gracias por tu interés en contribuir a **q-skillvault** (Qu@ntum)! Este proyecto es un sistema operativo de conocimiento local-first y fábrica issue-to-PR abierta a humanos y agentes autónomos.

- **Autor:** Gabriel Magallon Sanchez - Qu@antum
- **Licencia:** MIT ([LICENSE](LICENSE))
- **Repositorio:** [https://github.com/QuantumEdu/q-skillvault](https://github.com/QuantumEdu/q-skillvault)

---

## Tipos de Contribución y Clasificación

Las contribuciones a este ecosistema se dividen en 4 categorías principales:

```
                  q-skillvault Contributions
                              │
     ┌────────────────┬───────┴────────┬────────────────┐
     ▼                ▼                ▼                ▼
1. Skills &       2. Workflows      3. Core Go &     4. Docs &
   Agent Specs       Pipelines         Line Factory     Tutorials
```

### 1. Publicar un nuevo Skill (`SKILL.md`)
Un **Agent Skill** es un paquete de conocimiento reutilizable para agentes de IA (Claude Code, Cursor, Windsurf, Antigravity).

- **Estructura mínima requerida:**
  - Archivo `skills/<nombre-del-skill>/SKILL.md`.
  - Frontmatter YAML válido:
    ```yaml
    ---
    name: "q:mi-skill"
    description: "Trigger: /mi-skill, keywords... Qué hace el skill y cuándo debe activarlo el agente."
    version: 1.0.0
    license: MIT
    ---
    ```
  - Sin rutas personales hardcodeadas (usa variables de entorno o rutas relativas).
  - Bloques de código con etiquetas de lenguaje (`bash`, `json`, `go`, etc.).

- **Indexación automática:**
  - Una vez agregado en este repositorio, queda listo para su publicación e instalación inmediata en **`skills.sh`** con:
    ```bash
    npx skills add QuantumEdu/q-skillvault --skill q:mi-skill
    ```

### 2. Contribuir un Workflow (`workflow-builder`)
Flujos de trabajo estructurados de múltiples fases con criterios de aceptación:
- Los flujos se pueden importar con `skillvault import workflow.yaml` o definirse en `internal/domain/`.
- Deben clasificarse según la taxonomía **LifeOS** (`WORK`, `KNOWLEDGE`, `LEARNING`, `RELATIONSHIP`, `STATE`, `OBSERVABILITY`).

### 3. Código Core de Go (`skillvault`, `line`, `telemetryd`)
- **Regla de Cero CGO:** La base de código usa Go estándar y `modernc.org/sqlite`. No se admiten dependencias con CGO.
- **Clean Architecture:** Mantener separación estricta entre `internal/domain/`, `internal/db/`, `internal/api/` y `internal/cli/`.
- **Fábrica `line`:** Cambios en la orquestación FSM (`plan` → `build` → `review` → `wait_ci` → `ready`) o en el TUI interactivo de Bubble Tea (`internal/line/tui/`).
- **Pruebas:** Toda nueva funcionalidad o corrección debe incluir pruebas unitarias con cobertura (`go test ./...` debe pasar al 100%).

### 4. Documentación y Tutoriales
- Documentación técnica en [`docs/`](docs/).
- Tutoriales interactivos HTML con maquetación moderna (siguiendo los estándares de `docs/tutorial-line-factory.html`).

---

## Flujo de Desarrollo

### 1. Clonar con Submódulos
```bash
git clone --recurse-submodules https://github.com/QuantumEdu/q-skillvault.git
cd q-skillvault
```

### 2. Crear una Rama
```bash
git checkout -b feature/nombre-de-tu-mejora
# o fix/descripcion-del-bug
```

### 3. Verificar Pruebas y Compilación
```bash
# Ejecutar todas las pruebas unitarias
go test ./...

# Probar compilación de toda la suite
make install-all

# Limpieza
make clean
```

### 4. Enviar Pull Request
- Abre un Pull Request contra la rama `main`.
- Describe claramente qué problema resuelve o qué nuevo skill/workflow añade.
- Asegúrate de que las pruebas pasen en GitHub Actions CI.

---

## Guía de Issues

Para reportar problemas o sugerir nuevas capacidades, utiliza las plantillas disponibles en [Issues](https://github.com/QuantumEdu/q-skillvault/issues):
- **Bug Report**: Comportamiento inesperado o fallos de compilación.
- **Feature Request**: Propuestas de nuevas herramientas, modos de contexto o mejoras en `line`.
- **Skill / Workflow Submission**: Propuesta para integrar un nuevo skill o workflow en el vault.
