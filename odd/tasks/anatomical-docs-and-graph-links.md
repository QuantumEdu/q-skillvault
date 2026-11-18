# Feature: anatomical-docs-and-graph-links

- **Feature:** `anatomical-docs-and-graph-links`
- **Objective:** Actualizar documentación para reflejar la anatomía real de 8 capas de SkillVault, implementar `skillvault mem promote` y activar enlaces semánticos tipados en `entry_links`, bump de versión menor, pruebas funcionales + ejercicios en vivo, commit + PR + merge.
- **Plan:** B (Brownfield)
- **Why:** La descripción pública de SkillVault dice "Knowledge OS y compilador de contexto" pero no refleja los módulos de memoria episódica, telemetría, secretos, CLI de mem, etc. Los agentes externos leen esa descripción y subestiman el sistema.

## Specs

- **S-01** Actualizar SKILL.md raíz (`/q/devs/q-skillvault/SKILL.md`) con la descomposición anatómica de 8 capas (Interfaces de Acceso, 6 módulos funcionales, Almacenamiento Híbrido) y versión bump a `3.7.0`.
- **S-02** Actualizar `skills/q-skillvault/SKILL.md` (dentro del repositorio) con descripción expandida de los módulos: Memoria Episódica, Compilador de Contexto, Workflows, Productividad/Handoffs, Telemetría, Seguridad/Secretos.
- **S-03** Actualizar `docs/architecture.md` para incluir diagrama Mermaid de las 8 capas y la tabla de módulos funcionales.
- **S-04** Implementar `skillvault mem promote` que convierta observaciones episódicas frecuentes (`duplicate_count >= threshold`) en entradas formales de tipo `decision` o `skill`.
- **S-05** Activar `entry_links` con tipos semánticos (`contradicts`, `depends_on`, `cites`, `supersedes`, `part_of`) exponiéndolos desde el CLI vía `skillvault link-semantic` o ampliando `entry ref`.
- **S-06** Bump de versión a `3.7.0` en `internal/version/version.go`.
- **S-07** Tests unitarios para `mem promote` y para tipos semánticos en `entry_links`.
- **S-08** Ejercicios en vivo con vault real: demostrar `mem save → mem promote`, `entry ref add --rel depends_on`, `mem search` y `context bundle`.
- **S-09** Commit, push en rama feature, PR y merge.

## Tasks

- [x] **TASK-01: Doc patches — SKILL.md raíz + skills/q-skillvault/SKILL.md**
  - *Spec:* S-01, S-02
  - *Route:* inline
  - *Checks:* archivos editados, diff lógico verificado, versión 3.7.0 reflejada

- [x] **TASK-02: Doc patch — docs/architecture.md con diagrama Mermaid**
  - *Spec:* S-03
  - *Route:* inline
  - *Checks:* Mermaid válido renderizado, tabla de 8 capas funcionales añadida

- [x] **TASK-03: Implementar `skillvault mem promote`**
  - *Spec:* S-04
  - *Route:* inline
  - *Checks:* `go test ./internal/app/... ./internal/cli/...` PASS, `PromoteVaultMem` con gate de duplicados y preservación de linaje

- [x] **TASK-04: Activar tipos semánticos en `entry ref add`**
  - *Spec:* S-05
  - *Route:* inline
  - *Checks:* `RelationContradicts` y `RelationCites` añadidos a domain; flag `--rel` y `--type` soportados en CLI `entry ref add`; tests domain PASS

- [x] **TASK-05: Bump versión + tests + ejercicios en vivo**
  - *Spec:* S-06, S-07, S-08
  - *Route:* inline
  - *Checks:* `version.Number = "3.7.0"`, `go test ./...` 22/22 paquetes PASS, ejercicios en vivo ejecutados exitosamente (`mem save` + dedupe, `mem promote`, `entry ref add --rel depends_on`, `graph` Mermaid rendering)

- [ ] **TASK-06: Commit, push, PR y merge**
  - *Spec:* S-09
  - *Route:* inline
  - *Checks:* PR merged en origin/main

## Log

L1 (2026-11-18T08:22): Usuario solicita: (1) corregir documentación con la descomposición anatómica de SkillVault, (2) implementar usando q-agent, (3) ajustar versionado mínimo y documentación, (4) tests + ejercicios funcionales para verificar operación, (5) commit + push + PR + merge.
