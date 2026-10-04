---
name: skillvault-sync
description: >
  Synchronize, backup, and bidirectionally merge SkillVault SQLite databases and markdown entries across machines
  (WSL, Windows, macOS, Linux) using the private GitHub repository https://github.com/QuantumEdu/skill-vault-backp.
  Trigger: /skillvault-sync, sync skillvault, backup skillvault, respaldar skillvault, sincronizar skillvault.
license: Apache-2.0
metadata:
  author: QuantumEdu
  version: "1.0.0"
  date: "2026-10-04"
---

# SkillVault Sync & Bidirectional Merge Skill (`skillvault-sync`)

Este skill proporciona respaldo automático, sincronización bidireccional y unificación con resolución de colisiones (*Last-Write-Wins* por `updated_at`) para **SkillVault** (`~/.skillvault/vault.db`), en estricta conformidad con las convenciones del repositorio privado [QuantumEdu/skill-vault-backp](https://github.com/QuantumEdu/skill-vault-backp).

---

## Características Principales

1. **Snapshots Atómicos Consistentes**:
   - Utiliza `VACUUM INTO` sobre SQLite para evitar bloqueos del archivo WAL (`SQLITE_BUSY`).
2. **Reconciliación Bidireccional (True Two-Way Merge)**:
   - Combina la base activa local con la base remota del repositorio.
   - Entradas y proyectos nuevos en cualquiera de los dos lados se incorporan automáticamente.
   - En caso de conflicto en registros coincidentes, aplica **Last-Write-Wins (LWW)** según la fecha ISO de modificación (`updated_at`).
   - Unión de etiquetas (`entry_tags`) y enlaces de grafos (`entry_links`).
3. **Reconstrucción Automática de FTS5**:
   - Reconstruye automáticamente el índice virtual de búsqueda semántica y full-text (`INSERT INTO entries_fts(entries_fts) VALUES('rebuild')`) tras cada unificación.
4. **Catálogo Vivo y Legibilidad de Agentes**:
   - Exporta cada entrada a formato Markdown individual con frontmatter YAML en `data/entries/<slug>.md`.
   - Genera volcados JSON completos (`data/projects.json`, `data/entries.json`).
   - Regenera el inventario visual de tablas en `README.md` del repositorio de respaldo.
5. **Seguridad y Respaldo Pre-Apply**:
   - Antes de aplicar cualquier base de datos unificada, crea una copia de seguridad en `~/.skillvault/exports/pre-apply-backup-<timestamp>.db`.
   - Limpia sidecars temporales (`-wal`, `-shm`) para garantizar apertura limpia.

---

## Archivo de Configuración

Ubicación:
- **WSL / Linux**: `~/.config/skillvault-sync/config.env`

Variables configurables:
```env
# URL del repositorio de almacenamiento
SKILLVAULT_SYNC_REPO_URL="https://github.com/QuantumEdu/skill-vault-backp.git"

# Ubicación del clon local del repositorio de respaldo
SKILLVAULT_SYNC_REPO_DIR="~/.local/share/skillvault-sync/repo"

# Rama de sincronización
SKILLVAULT_SYNC_BRANCH="main"

# Identificador de la máquina (wsl-ubuntu, macos, etc.)
SKILLVAULT_MACHINE_NAME="wsl-ubuntu"

# Ruta de la base de datos activa
SKILLVAULT_DB_PATH="~/.skillvault/vault.db"

# Push automático a GitHub tras snapshot o sync
SKILLVAULT_AUTO_PUSH=true

# Identidad del autor de commits en el repo de respaldo
SKILLVAULT_GIT_USER_NAME="Gabriel Magallon Sanchez"
SKILLVAULT_GIT_USER_EMAIL="gmagallon@mich.conalep.edu.mx"
```

---

## Comandos Disponibles

El comando CLI `skillvault-sync` está disponible en el `$PATH`:

```bash
# 1. Ver estado comparativo (local vs repositorio remoto)
skillvault-sync status

# 2. Inicializar configuración y clonar el repositorio de respaldo
skillvault-sync init

# 3. Descargar cambios recientes de GitHub (git pull)
skillvault-sync pull

# 4. Sincronización bidireccional completa (Pull + Merge + Apply + Snapshot + Push)
skillvault-sync sync [--note "nota explicativa"]

# 5. Generar snapshot local y subir al repositorio (sin mezclar)
skillvault-sync snapshot [--note "respaldo manual"] [--no-push]

# 6. Unificar bases de datos en archivo temporal
skillvault-sync unify [--out /tmp/unified.db]

# 7. Aplicar base de datos unificada a la activa
skillvault-sync apply --unified /tmp/unified.db
```

---

## Flujo de Trabajo Recomendado

### En el Cierre de Sesión (`q:session-wrap`)
Al terminar una jornada de desarrollo o invocar `/q-session-wrap`:
```bash
skillvault-sync sync --note "cierre de sesion"
```
Ambos extremos (tu base local y [QuantumEdu/skill-vault-backp](https://github.com/QuantumEdu/skill-vault-backp)) quedan sincronizados al 100%.

### Al Cambiar de Máquina (Ej. de WSL a otra máquina)
1. Ejecutar `skillvault-sync init` en la nueva máquina.
2. Ejecutar `skillvault-sync apply --unified ~/.local/share/skillvault-sync/repo/vault.db`.
3. Tu entorno local contará inmediatamente con todas tus notas, proyectos, links y recetas de comandos.
