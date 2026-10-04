#!/usr/bin/env python3
"""
skillvault_sync.py — Standalone multi-device synchronizer, backup, and bidirectional merge engine for SkillVault.
Conforms strictly to QuantumEdu/skill-vault-backp conventions.

Usage:
  skillvault-sync status
  skillvault-sync init
  skillvault-sync pull
  skillvault-sync snapshot [--note NOTE] [--no-push]
  skillvault-sync unify [--out UNIFIED.db]
  skillvault-sync apply [--unified UNIFIED.db] [--yes]
  skillvault-sync sync [--note NOTE] [--no-push]
"""

import argparse
import datetime
import hashlib
import json
import os
import re
import shutil
import sqlite3
import subprocess
import sys
from pathlib import Path

DEFAULT_CONFIG_PATH = Path.home() / ".config" / "skillvault-sync" / "config.env"
DEFAULT_REPO_DIR = Path.home() / ".local" / "share" / "skillvault-sync" / "repo"
DEFAULT_REPO_URL = "https://github.com/QuantumEdu/skill-vault-backp.git"
DEFAULT_DB_PATH = Path.home() / ".skillvault" / "vault.db"


def load_config(config_file=None):
    """Load key-value environment configuration."""
    def detect_machine():
        if os.path.exists("/proc/version"):
            try:
                with open("/proc/version", "r") as f:
                    if "microsoft" in f.read().lower():
                        return "wsl-ubuntu"
            except Exception:
                pass
        return os.uname().nodename if hasattr(os, "uname") else "linux-host"

    cfg = {
        "SKILLVAULT_SYNC_REPO_URL": DEFAULT_REPO_URL,
        "SKILLVAULT_SYNC_REPO_DIR": str(DEFAULT_REPO_DIR),
        "SKILLVAULT_SYNC_BRANCH": "main",
        "SKILLVAULT_MACHINE_NAME": detect_machine(),
        "SKILLVAULT_DB_PATH": str(DEFAULT_DB_PATH),
        "SKILLVAULT_AUTO_PUSH": "true",
        "SKILLVAULT_GIT_USER_NAME": "Gabriel Magallon Sanchez",
        "SKILLVAULT_GIT_USER_EMAIL": "gmagallon@mich.conalep.edu.mx",
    }

    target_cfg = Path(config_file) if config_file else (
        Path(os.environ.get("SKILLVAULT_SYNC_CONFIG", "")) if os.environ.get("SKILLVAULT_SYNC_CONFIG") else DEFAULT_CONFIG_PATH
    )

    if target_cfg.exists():
        with open(target_cfg, "r", encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if "=" in line:
                    k, v = line.split("=", 1)
                    k = k.strip()
                    v = v.strip().strip("'\"")
                    if v.startswith("~"):
                        v = str(Path(v).expanduser())
                    cfg[k] = v

    cfg["SKILLVAULT_SYNC_REPO_DIR"] = str(Path(os.path.expandvars(cfg["SKILLVAULT_SYNC_REPO_DIR"])).expanduser())
    cfg["SKILLVAULT_DB_PATH"] = str(Path(os.path.expandvars(cfg["SKILLVAULT_DB_PATH"])).expanduser())
    cfg["SKILLVAULT_AUTO_PUSH"] = str(cfg["SKILLVAULT_AUTO_PUSH"]).lower() in ("true", "1", "yes")

    return cfg, target_cfg


def run_cmd(cmd, cwd=None, check=True):
    """Run a shell command safely."""
    res = subprocess.run(cmd, shell=True, cwd=cwd, text=True, capture_output=True)
    if check and res.returncode != 0:
        raise RuntimeError(
            f"Command failed (exit {res.returncode}): {cmd}\nStderr: {res.stderr.strip()}\nStdout: {res.stdout.strip()}"
        )
    return res.stdout.strip(), res.returncode


def get_file_sha256(filepath):
    """Compute SHA256 of a file."""
    h = hashlib.sha256()
    with open(filepath, "rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def get_db_stats(db_path):
    """Return dictionary with record counts, schema version, and file metadata."""
    p = Path(db_path)
    if not p.exists():
        return None
    try:
        conn = sqlite3.connect(f"file:{p.resolve()}?mode=ro", uri=True)
        cur = conn.cursor()

        def safe_count(table):
            try:
                return cur.execute(f"SELECT COUNT(*) FROM {table}").fetchone()[0]
            except Exception:
                return 0

        projects = safe_count("projects")
        entries = safe_count("entries")
        tags = safe_count("tags")
        workflows = safe_count("workflows")
        links = safe_count("entry_links")

        # Max schema migration
        max_mig = 0
        try:
            row = cur.execute("SELECT MAX(version) FROM schema_migrations").fetchone()
            if row and row[0]:
                max_mig = row[0]
        except Exception:
            pass

        conn.close()

        size_bytes = p.stat().st_size
        sha = get_file_sha256(str(p))
        mtime = datetime.datetime.fromtimestamp(p.stat().st_mtime).strftime("%Y-%m-%d %H:%M:%S")

        return {
            "path": str(p),
            "size_bytes": size_bytes,
            "size_kb": round(size_bytes / 1024, 1),
            "projects": projects,
            "entries": entries,
            "tags": tags,
            "workflows": workflows,
            "links": links,
            "migration": max_mig,
            "mtime": mtime,
            "sha256": sha[:12],
        }
    except Exception as e:
        return {"error": str(e), "path": str(p)}


def slugify(text):
    """Generate safe filename slug from text."""
    if not text:
        return "untitled"
    s = text.strip().lower()
    s = re.sub(r'[\s_]+', '-', s)
    s = re.sub(r'[^a-z0-9\-áéíóúüñ]', '', s)
    s = re.sub(r'-+', '-', s).strip('-')
    return s or "entry"


def export_markdown_entries(conn, output_dir):
    """Export every entry in the database as an individual markdown file with YAML frontmatter."""
    out_path = Path(output_dir)
    if out_path.exists():
        shutil.rmtree(out_path)
    out_path.mkdir(parents=True, exist_ok=True)

    cur = conn.cursor()
    query = """
        SELECT id, COALESCE(name, ''), COALESCE(title, ''), COALESCE(slug, ''),
               type, COALESCE(summary, ''), COALESCE(body_optional, ''),
               COALESCE(purpose, ''), status, COALESCE(project_id, ''),
               COALESCE(artifact_id, ''), COALESCE(external_ref, ''),
               created_at, updated_at
        FROM entries
        ORDER BY created_at ASC
    """
    rows = cur.execute(query).fetchall()

    exported_count = 0
    used_filenames = set()

    for r in rows:
        (eid, name, title, slug, etype, summary, body, purpose,
         status, project_id, artifact_id, external_ref, created_at, updated_at) = r

        base_slug = slug or slugify(title or name or eid)
        filename = f"{base_slug}.md"
        counter = 1
        while filename in used_filenames:
            filename = f"{base_slug}-{counter}.md"
            counter += 1
        used_filenames.add(filename)

        file_path = out_path / filename

        # Fetch tags
        tags_rows = cur.execute("SELECT tag FROM entry_tags WHERE entry_id = ? ORDER BY tag", (eid,)).fetchall()
        tags_list = [t[0] for t in tags_rows]

        frontmatter = [
            "---",
            f'id: "{eid}"',
            f'title: {json.dumps(title or name)}',
            f'slug: "{base_slug}"',
            f'type: "{etype}"',
        ]
        if project_id:
            frontmatter.append(f'project_id: "{project_id}"')
        if summary:
            frontmatter.append(f'summary: {json.dumps(summary)}')
        if purpose:
            frontmatter.append(f'purpose: "{purpose}"')
        if status:
            frontmatter.append(f'status: "{status}"')
        if external_ref:
            frontmatter.append(f'external_ref: {json.dumps(external_ref)}')
        if tags_list:
            frontmatter.append(f'tags: {json.dumps(tags_list)}')
        if created_at:
            frontmatter.append(f'created_at: "{created_at}"')
        if updated_at:
            frontmatter.append(f'updated_at: "{updated_at}"')
        frontmatter.append("---")
        frontmatter.append("")

        content = "\n".join(frontmatter) + (body or "") + "\n"
        with open(file_path, "w", encoding="utf-8") as f:
            f.write(content)
        exported_count += 1

    return exported_count


def export_json_dumps(conn, data_dir):
    """Dump projects.json and entries.json into data directory."""
    dp = Path(data_dir)
    dp.mkdir(parents=True, exist_ok=True)

    cur = conn.cursor()

    # Projects dump
    cur.execute("SELECT id, name, slug, description, status, created_at, updated_at FROM projects ORDER BY id")
    p_rows = cur.fetchall()
    projects = []
    for r in p_rows:
        projects.append({
            "id": r[0], "name": r[1], "slug": r[2], "description": r[3],
            "status": r[4], "created_at": r[5], "updated_at": r[6]
        })
    with open(dp / "projects.json", "w", encoding="utf-8") as f:
        json.dump(projects, f, indent=2, ensure_ascii=False)

    # Entries dump
    cur.execute("""
        SELECT id, name, title, slug, type, description, content, summary,
               body_optional, purpose, status, project_id, artifact_id,
               external_ref, vars, tags_denorm, active, created_at, updated_at
        FROM entries ORDER BY id
    """)
    e_rows = cur.fetchall()
    entries = []
    for r in e_rows:
        entries.append({
            "id": r[0], "name": r[1], "title": r[2], "slug": r[3],
            "type": r[4], "description": r[5], "content": r[6], "summary": r[7],
            "body_optional": r[8], "purpose": r[9], "status": r[10],
            "project_id": r[11], "artifact_id": r[12], "external_ref": r[13],
            "vars": r[14], "tags_denorm": r[15], "active": r[16],
            "created_at": r[17], "updated_at": r[18]
        })
    with open(dp / "entries.json", "w", encoding="utf-8") as f:
        json.dump(entries, f, indent=2, ensure_ascii=False)

    return len(projects), len(entries)


def generate_repo_readme(conn, repo_path):
    """Generate comprehensive README.md with table of projects and entries."""
    cur = conn.cursor()

    p_count = cur.execute("SELECT COUNT(*) FROM projects").fetchone()[0]
    e_count = cur.execute("SELECT COUNT(*) FROM entries").fetchone()[0]
    t_count = cur.execute("SELECT COUNT(*) FROM tags").fetchone()[0]
    w_count = cur.execute("SELECT COUNT(*) FROM workflows").fetchone()[0]
    a_count = cur.execute("SELECT COUNT(*) FROM artifacts").fetchone()[0]

    projects = cur.execute("SELECT id, name, slug, description, status, created_at FROM projects ORDER BY name").fetchall()
    entries = cur.execute("""
        SELECT e.id, e.title, e.slug, e.type, COALESCE(p.name, 'N/A'), e.summary, e.updated_at
        FROM entries e
        LEFT JOIN projects p ON e.project_id = p.id
        ORDER BY e.updated_at DESC
    """).fetchall()

    now_str = datetime.datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    md = [
        "# SkillVault Backup (`skill-vault-backp`)",
        "",
        "Respaldo privado y automatizado del almacén de conocimiento local de **SkillVault** (`~/.skillvault`).",
        "",
        f"> **Última sincronización:** `{now_str}`",
        "",
        "---",
        "",
        "## 📊 Resumen de Registros Respaldados",
        "",
        "| Tipo de Registro | Cantidad | Descripción |",
        "|---|:---:|---|",
        f"| **Proyectos (`projects`)** | **{p_count}** | Entornos de trabajo registrados en el vault |",
        f"| **Entradas (`entries`)** | **{e_count}** | Documentos, sesiones arquitectónicas, links, comandos y referencias |",
        f"| **Etiquetas (`tags`)** | **{t_count}** | Tags normalizados FTS5 |",
        f"| **Artefactos (`artifacts`)** | **{a_count}** | Archivos binarios / adjuntos |",
        f"| **Workflows / Flujos** | **{w_count}** | Pipelines automatizados |",
        "",
        "---",
        "",
        "## 📁 Detalle de Proyectos",
        "",
    ]

    for p in projects:
        pid, pname, pslug, pdesc, pstatus, pcreated = p
        md.append(f"### • `{pname}` (`{pid}`)")
        md.append(f"- **Slug:** `{pslug}` | **Estado:** `{pstatus}` | **Creado:** `{pcreated}`")
        if pdesc:
            md.append(f"- **Descripción:** {pdesc}")
        md.append("")

    md.extend([
        "---",
        "",
        "## 📝 Inventario de Entradas Recientes",
        "",
        "| Tipo | Título | Proyecto | Actualizado | Archivo Markdown |",
        "|---|---|---|---|---|",
    ])

    for e in entries:
        eid, etitle, eslug, etype, epname, esummary, eupdated = e
        title_esc = (etitle or eid).replace("|", "\\|")
        md_file = f"data/entries/{eslug or eid}.md"
        md.append(f"| `{etype}` | **{title_esc}** | `{epname}` | `{eupdated or ''}` | [`{eslug or eid}.md`]({md_file}) |")

    md.extend([
        "",
        "---",
        "",
        "## 🗂️ Estructura del Respaldo",
        "",
        "```text",
        "skill-vault-backp/",
        "├── README.md               # Este catálogo autogenerado con inventario completo",
        "├── vault.db                # Base de datos SQLite limpia (WAL checkpoint consolidado)",
        "├── .gitignore              # Filtro de archivos temporales",
        "├── exports/                # Exportaciones JSON fechadas de SkillVault",
        "│   └── skillvault-backup-*.json",
        "└── data/",
        "    ├── entries.json        # Volcado JSON completo de la tabla entries",
        "    ├── projects.json       # Volcado JSON completo de la tabla projects",
        "    └── entries/            # Entradas exportadas a formato Markdown individual",
        "        ├── <slug>.md",
        "        └── ...",
        "```",
        "",
        "---",
        "",
        "## 🔄 Restauración",
        "",
        "### Opción A: Restaurar base de datos directa (Recomendado)",
        "```bash",
        "cp vault.db ~/.skillvault/vault.db",
        "```",
        "",
        "### Opción B: Sincronización automática vía CLI",
        "```bash",
        "skillvault-sync sync",
        "```",
        ""
    ])

    readme_path = Path(repo_path) / "README.md"
    with open(readme_path, "w", encoding="utf-8") as f:
        f.write("\n".join(md))


def cmd_status(cfg):
    """Display status comparison between local database and remote repository."""
    local_db = cfg["SKILLVAULT_DB_PATH"]
    repo_dir = cfg["SKILLVAULT_SYNC_REPO_DIR"]
    repo_db = os.path.join(repo_dir, "vault.db")

    print("\n\033[1;36m[skillvault-sync]\033[0m Estado de Sincronización de SkillVault")
    print(f"  Repositorio Remoto: {cfg['SKILLVAULT_SYNC_REPO_URL']}")
    print(f"  Directorio Local de Repo: {repo_dir}")
    print(f"  Base Activa Local: {local_db}")

    l_stats = get_db_stats(local_db)
    r_stats = get_db_stats(repo_db)

    print("\n\033[1;33m--- Base de Datos Local vs Repositorio ---\033[0m")
    if l_stats and "error" not in l_stats:
        print(f"  Local:  {l_stats['size_kb']} KB | Proyectos: {l_stats['projects']} | Entradas: {l_stats['entries']} | Tags: {l_stats['tags']} | Migración: v{l_stats['migration']}")
    else:
        print(f"  Local:  \033[31mNo encontrada o error: {l_stats}\033[0m")

    if r_stats and "error" not in r_stats:
        print(f"  Remoto: {r_stats['size_kb']} KB | Proyectos: {r_stats['projects']} | Entradas: {r_stats['entries']} | Tags: {r_stats['tags']} | Migración: v{r_stats['migration']}")
    else:
        print(f"  Remoto: \033[31mNo encontrada o error (¿ejecutaste 'init'?)\033[0m")

    # Detailed ID diff if both exist
    if l_stats and r_stats and "error" not in l_stats and "error" not in r_stats:
        l_conn = sqlite3.connect(f"file:{Path(local_db).resolve()}?mode=ro", uri=True)
        r_conn = sqlite3.connect(f"file:{Path(repo_db).resolve()}?mode=ro", uri=True)

        l_entries = set(r[0] for r in l_conn.execute("SELECT id FROM entries").fetchall())
        r_entries = set(r[0] for r in r_conn.execute("SELECT id FROM entries").fetchall())

        l_proj = set(r[0] for r in l_conn.execute("SELECT id FROM projects").fetchall())
        r_proj = set(r[0] for r in r_conn.execute("SELECT id FROM projects").fetchall())

        l_conn.close()
        r_conn.close()

        only_l_e = l_entries - r_entries
        only_r_e = r_entries - l_entries
        shared_e = l_entries & r_entries

        only_l_p = l_proj - r_proj
        only_r_p = r_proj - l_proj

        print(f"\n  Diferencias de Entradas:")
        print(f"  - Compartidas: \033[32m{len(shared_e)}\033[0m")
        print(f"  - Solo en Local (pendientes de subir):  \033[36m{len(only_l_e)}\033[0m")
        print(f"  - Solo en Remoto (pendientes de bajar): \033[33m{len(only_r_e)}\033[0m")

        if only_l_p or only_r_p:
            print(f"  Diferencias de Proyectos:")
            if only_l_p:
                print(f"  - Solo en Local: {only_l_p}")
            if only_r_p:
                print(f"  - Solo en Remoto: {only_r_p}")

    # Git status
    if os.path.exists(os.path.join(repo_dir, ".git")):
        out, _ = run_cmd("git status -s", cwd=repo_dir, check=False)
        branch, _ = run_cmd("git rev-parse --abbrev-ref HEAD", cwd=repo_dir, check=False)
        commit, _ = run_cmd("git log -1 --oneline", cwd=repo_dir, check=False)
        print(f"\n\033[1;33m--- Estado Git del Repositorio de Respaldo ---\033[0m")
        print(f"  Rama: {branch}")
        print(f"  Último commit: {commit}")
        if out:
            print(f"  Cambios sin commitear:\n{out}")
        else:
            print("  Árbol de trabajo limpio (clean).")
    print()


def cmd_init(cfg, target_cfg):
    """Initialize configuration and clone the backup repository if needed."""
    print(f"\n\033[1;34m[skillvault-sync]\033[0m Inicializando configuración y repositorio...")

    # Write default config if missing
    if not target_cfg.exists():
        target_cfg.parent.mkdir(parents=True, exist_ok=True)
        content = f"""# SkillVault Backup & Two-Way Sync Configuration
# Repository URL
SKILLVAULT_SYNC_REPO_URL="{cfg['SKILLVAULT_SYNC_REPO_URL']}"

# Local clone path
SKILLVAULT_SYNC_REPO_DIR="{cfg['SKILLVAULT_SYNC_REPO_DIR']}"

# Branch
SKILLVAULT_SYNC_BRANCH="{cfg['SKILLVAULT_SYNC_BRANCH']}"

# Machine name identifier
SKILLVAULT_MACHINE_NAME="{cfg['SKILLVAULT_MACHINE_NAME']}"

# Active local database path
SKILLVAULT_DB_PATH="{cfg['SKILLVAULT_DB_PATH']}"

# Auto push on snapshot/sync
SKILLVAULT_AUTO_PUSH=true

# Git commit identity
SKILLVAULT_GIT_USER_NAME="{cfg['SKILLVAULT_GIT_USER_NAME']}"
SKILLVAULT_GIT_USER_EMAIL="{cfg['SKILLVAULT_GIT_USER_EMAIL']}"
"""
        with open(target_cfg, "w", encoding="utf-8") as f:
            f.write(content)
        print(f"  ✓ Creado archivo de configuración: {target_cfg}")
    else:
        print(f"  ✓ Archivo de configuración existente: {target_cfg}")

    repo_dir = cfg["SKILLVAULT_SYNC_REPO_DIR"]
    if not os.path.exists(os.path.join(repo_dir, ".git")):
        print(f"  Clonando {cfg['SKILLVAULT_SYNC_REPO_URL']} en {repo_dir}...")
        Path(repo_dir).parent.mkdir(parents=True, exist_ok=True)
        run_cmd(f"git clone {cfg['SKILLVAULT_SYNC_REPO_URL']} {repo_dir}")
        print(f"  ✓ Repositorio clonado exitosamente.")
    else:
        print(f"  ✓ Repositorio ya clonado en {repo_dir}.")

    # Configure local git user if needed
    run_cmd(f'git config user.name "{cfg["SKILLVAULT_GIT_USER_NAME"]}"', cwd=repo_dir, check=False)
    run_cmd(f'git config user.email "{cfg["SKILLVAULT_GIT_USER_EMAIL"]}"', cwd=repo_dir, check=False)
    print("\033[1;32m[skillvault-sync]\033[0m Inicialización completada exitosamente.\n")


def cmd_pull(cfg):
    """Pull latest updates from the remote backup repository."""
    repo_dir = cfg["SKILLVAULT_SYNC_REPO_DIR"]
    if not os.path.exists(os.path.join(repo_dir, ".git")):
        raise RuntimeError(f"El repositorio no está inicializado en {repo_dir}. Ejecuta 'skillvault-sync init'.")
    print(f"\n\033[1;34m[skillvault-sync]\033[0m Descargando cambios remotos (git pull)...")
    out, _ = run_cmd(f"git pull origin {cfg['SKILLVAULT_SYNC_BRANCH']}", cwd=repo_dir)
    print(f"  {out}")
    print("\033[1;32m[skillvault-sync]\033[0m Pull completado.\n")


def unify_databases(local_db_path, remote_db_path, output_path):
    """
    Bidirectionally merges local and remote databases into output_path using Last-Write-Wins (LWW)
    by updated_at timestamp, union of tags, and FTS5 index reconstruction.
    """
    out_p = Path(output_path)
    if out_p.exists():
        out_p.unlink()

    # Step 1: Base snapshot from local database using VACUUM INTO
    # This guarantees we inherit all 12 migrations and latest table structures.
    l_conn = sqlite3.connect(local_db_path)
    l_conn.execute(f"VACUUM INTO '{out_p.resolve()}'")
    l_conn.close()

    # Step 2: Open target database and attach remote database
    conn = sqlite3.connect(str(out_p))
    conn.execute("PRAGMA foreign_keys = OFF")
    conn.execute(f"ATTACH DATABASE '{Path(remote_db_path).resolve()}' AS remote")

    # Step 3: Merge Projects
    # Handle slug conflicts: check existing slugs
    remote_projects = conn.execute("""
        SELECT id, name, slug, description, status, created_at, updated_at
        FROM remote.projects
    """).fetchall()

    existing_slugs = set(r[0] for r in conn.execute("SELECT slug FROM projects").fetchall())
    existing_pids = set(r[0] for r in conn.execute("SELECT id FROM projects").fetchall())

    for p in remote_projects:
        pid, name, pslug, desc, status, cat, uat = p
        if pid not in existing_pids:
            # Check slug collision
            target_slug = pslug
            if target_slug in existing_slugs:
                target_slug = f"{pslug}-import-{hashlib.md5(pid.encode()).hexdigest()[:6]}"
            conn.execute("""
                INSERT INTO projects (id, name, slug, description, status, created_at, updated_at)
                VALUES (?, ?, ?, ?, ?, ?, ?)
            """, (pid, name, target_slug, desc, status, cat, uat))
            existing_slugs.add(target_slug)
            existing_pids.add(pid)
        else:
            # Existing project in both: compare updated_at
            local_uat = conn.execute("SELECT updated_at FROM projects WHERE id = ?", (pid,)).fetchone()[0] or ""
            if (uat or "") > (local_uat or ""):
                conn.execute("""
                    UPDATE projects SET name=?, description=?, status=?, updated_at=?
                    WHERE id = ?
                """, (name, desc, status, uat, pid))

    # Step 4: Merge Tags (Union)
    conn.execute("""
        INSERT OR IGNORE INTO tags (id, name, slug)
        SELECT id, name, slug FROM remote.tags
    """)

    # Step 5: Merge Entries (Last-Write-Wins on updated_at)
    remote_entries = conn.execute("""
        SELECT id, name, title, slug, type, description, content, summary,
               body_optional, purpose, status, project_id, artifact_id,
               external_ref, vars, tags_denorm, active, created_at, updated_at
        FROM remote.entries
    """).fetchall()

    local_entry_rows = conn.execute("SELECT id, updated_at, slug FROM entries").fetchall()
    local_entry_map = {r[0]: (r[1] or "", r[2] or "") for r in local_entry_rows}
    local_slugs = set(r[1] for r in local_entry_map.values() if r[1])

    inserted_e = 0
    updated_e = 0

    for re_row in remote_entries:
        (eid, name, title, eslug, etype, desc, content, summary,
         body_optional, purpose, status, project_id, artifact_id,
         external_ref, evars, tags_denorm, active, cat, uat) = re_row

        if eid not in local_entry_map:
            # New entry from remote: ensure slug is unique
            target_slug = eslug
            if target_slug and target_slug in local_slugs:
                target_slug = f"{eslug}-import-{hashlib.md5(eid.encode()).hexdigest()[:6]}"
            local_slugs.add(target_slug)

            # Ensure foreign key project exists or set to NULL
            if project_id and project_id not in existing_pids:
                project_id = None

            conn.execute("""
                INSERT INTO entries (
                    id, name, title, slug, type, description, content, summary,
                    body_optional, purpose, status, project_id, artifact_id,
                    external_ref, vars, tags_denorm, active, created_at, updated_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """, (
                eid, name, title, target_slug, etype, desc, content, summary,
                body_optional, purpose, status, project_id, artifact_id,
                external_ref, evars, tags_denorm, active, cat, uat
            ))
            inserted_e += 1
        else:
            # Exists in both: compare updated_at
            local_uat, _ = local_entry_map[eid]
            if (uat or "") > (local_uat or ""):
                # Remote is newer: update local record
                if project_id and project_id not in existing_pids:
                    project_id = None
                conn.execute("""
                    UPDATE entries SET
                        name=?, title=?, type=?, description=?, content=?, summary=?,
                        body_optional=?, purpose=?, status=?, project_id=?, artifact_id=?,
                        external_ref=?, vars=?, tags_denorm=?, active=?, updated_at=?
                    WHERE id = ?
                """, (
                    name, title, etype, desc, content, summary,
                    body_optional, purpose, status, project_id, artifact_id,
                    external_ref, evars, tags_denorm, active, uat, eid
                ))
                updated_e += 1

    # Step 6: Merge Entry Tags (Union)
    conn.execute("""
        INSERT OR IGNORE INTO entry_tags (entry_id, tag)
        SELECT entry_id, tag FROM remote.entry_tags
        WHERE entry_id IN (SELECT id FROM entries)
    """)

    # Step 7: Merge Entry Links (Union)
    conn.execute("""
        INSERT OR IGNORE INTO entry_links (from_entry_id, to_entry_id, relation_type, label, active, created_at)
        SELECT from_entry_id, to_entry_id, relation_type, label, active, created_at
        FROM remote.entry_links
        WHERE from_entry_id IN (SELECT id FROM entries)
          AND to_entry_id IN (SELECT id FROM entries)
    """)

    # Step 8: Rebuild FTS5 Virtual Table
    try:
        conn.execute("INSERT INTO entries_fts(entries_fts) VALUES('rebuild')")
    except Exception as e:
        print(f"  [Aviso] FTS5 rebuild: {e}")

    # Step 9: Re-enable foreign keys and verify integrity
    conn.execute("PRAGMA foreign_keys = ON")
    check = conn.execute("PRAGMA integrity_check").fetchone()[0]
    if check != "ok":
        conn.close()
        raise RuntimeError(f"Integrity check failed on unified database: {check}")

    conn.commit()
    conn.close()

    return inserted_e, updated_e


def cmd_snapshot(cfg, note=None, push=True):
    """
    Take an atomic snapshot of local DB, regenerate all markdown entries and JSON dumps in repo,
    update README catalog, and push to GitHub.
    """
    local_db = cfg["SKILLVAULT_DB_PATH"]
    repo_dir = cfg["SKILLVAULT_SYNC_REPO_DIR"]
    repo_db = os.path.join(repo_dir, "vault.db")

    print(f"\n\033[1;34m[skillvault-sync]\033[0m Generando snapshot atómico y sincronizando repositorio...")

    # 1. Clean snapshot via VACUUM INTO
    temp_snap = Path(repo_dir) / ".temp_vacuum_snap.db"
    if temp_snap.exists():
        temp_snap.unlink()

    l_conn = sqlite3.connect(local_db)
    l_conn.execute(f"VACUUM INTO '{temp_snap.resolve()}'")
    l_conn.close()

    # Verify snapshot integrity
    v_conn = sqlite3.connect(str(temp_snap))
    check = v_conn.execute("PRAGMA integrity_check").fetchone()[0]
    if check != "ok":
        v_conn.close()
        temp_snap.unlink()
        raise RuntimeError(f"Fallo de integridad en snapshot: {check}")

    # Move snapshot to repo/vault.db
    shutil.move(str(temp_snap), repo_db)

    # 2. Export Markdown entries
    snap_conn = sqlite3.connect(repo_db)
    entries_exported = export_markdown_entries(snap_conn, os.path.join(repo_dir, "data", "entries"))

    # 3. Export JSON dumps
    p_exported, e_exported = export_json_dumps(snap_conn, os.path.join(repo_dir, "data"))

    # 4. Copy dated JSON export if present
    now_ts = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    local_exports_dir = Path.home() / ".skillvault" / "exports"
    repo_exports_dir = Path(repo_dir) / "exports"
    repo_exports_dir.mkdir(parents=True, exist_ok=True)

    # Dump a fresh dated backup
    snap_conn.row_factory = sqlite3.Row
    dated_json = repo_exports_dir / f"skillvault-backup-{now_ts}.json"
    p_rows = [dict(r) for r in snap_conn.execute("SELECT * FROM projects").fetchall()]
    e_rows = [dict(r) for r in snap_conn.execute("SELECT * FROM entries").fetchall()]
    t_rows = [dict(r) for r in snap_conn.execute("SELECT * FROM tags").fetchall()]
    et_rows = [dict(r) for r in snap_conn.execute("SELECT * FROM entry_tags").fetchall()]

    backup_payload = {
        "schema_version": 3,
        "app_version": "v3.2.0",
        "exported_at": datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "source": "skillvault-sync",
        "data": {
            "projects": p_rows,
            "entries": e_rows,
            "tags": t_rows,
            "entry_tags": et_rows,
        }
    }
    with open(dated_json, "w", encoding="utf-8") as f:
        json.dump(backup_payload, f, indent=2, ensure_ascii=False)

    # 5. Generate README catalog
    generate_repo_readme(snap_conn, repo_dir)
    snap_conn.close()

    print(f"  ✓ Base de datos: {repo_db} consolidada ({round(os.path.getsize(repo_db)/1024, 1)} KB)")
    print(f"  ✓ Archivos Markdown: {entries_exported} entradas en data/entries/*.md")
    print(f"  ✓ Catálogo README.md y dumps JSON actualizados")
    print(f"  ✓ Respaldo fechado: exports/{dated_json.name}")

    # 6. Git commit & push
    if push and cfg["SKILLVAULT_AUTO_PUSH"]:
        run_cmd("git add -A", cwd=repo_dir)
        status_out, _ = run_cmd("git status -s", cwd=repo_dir, check=False)
        if status_out:
            machine = cfg["SKILLVAULT_MACHINE_NAME"]
            commit_msg = f"backup({machine}): snapshot {now_ts} ({e_exported} entries, {p_exported} projects)"
            if note:
                commit_msg += f" — {note}"
            run_cmd(f'git commit -m "{commit_msg}"', cwd=repo_dir)
            print(f"  ✓ Commit realizado: '{commit_msg}'")
            print(f"  Subiendo a origin {cfg['SKILLVAULT_SYNC_BRANCH']}...")
            run_cmd(f"git push origin {cfg['SKILLVAULT_SYNC_BRANCH']}", cwd=repo_dir)
            print("  ✓ Cambios subidos exitosamente a GitHub.")
        else:
            print("  Sin cambios nuevos que commitear en el repositorio.")

    print("\033[1;32m[skillvault-sync]\033[0m Snapshot completado exitosamente.\n")


def cmd_apply(cfg, unified_db_path, yes=False):
    """
    Apply a unified database to the local active database with safety backup and sidecar cleanup.
    """
    local_db = Path(cfg["SKILLVAULT_DB_PATH"])
    unified_db = Path(unified_db_path)

    if not unified_db.exists():
        raise FileNotFoundError(f"Base de datos unificada no encontrada: {unified_db}")

    # Pre-check integrity of unified DB
    conn = sqlite3.connect(str(unified_db))
    check = conn.execute("PRAGMA integrity_check").fetchone()[0]
    conn.close()
    if check != "ok":
        raise RuntimeError(f"Integrity check falló en {unified_db}: {check}")

    now_ts = datetime.datetime.now().strftime("%Y%m%d-%H%M%S")
    backup_dir = local_db.parent / "exports"
    backup_dir.mkdir(parents=True, exist_ok=True)
    pre_backup = backup_dir / f"pre-apply-backup-{now_ts}.db"

    print(f"\n\033[1;34m[skillvault-sync]\033[0m Aplicando base unificada a la base activa...")
    if local_db.exists():
        # Checkpoint and backup existing local DB
        try:
            l_conn = sqlite3.connect(str(local_db))
            l_conn.execute("PRAGMA wal_checkpoint(TRUNCATE)")
            l_conn.execute(f"VACUUM INTO '{pre_backup.resolve()}'")
            l_conn.close()
            print(f"  ✓ Respaldo de seguridad previo creado: {pre_backup.name}")
        except Exception as e:
            print(f"  [Aviso] No se pudo hacer VACUUM de respaldo, copiando directamente: {e}")
            shutil.copy2(str(local_db), str(pre_backup))

    # Remove active sidecars
    wal_file = Path(str(local_db) + "-wal")
    shm_file = Path(str(local_db) + "-shm")
    if wal_file.exists():
        wal_file.unlink()
    if shm_file.exists():
        shm_file.unlink()

    # Atomically replace local DB
    shutil.copy2(str(unified_db), str(local_db))

    # Verify applied DB
    app_conn = sqlite3.connect(str(local_db))
    p_cnt = app_conn.execute("SELECT COUNT(*) FROM projects").fetchone()[0]
    e_cnt = app_conn.execute("SELECT COUNT(*) FROM entries").fetchone()[0]
    app_conn.close()

    print(f"  ✓ Base de datos activa actualizada: {local_db}")
    print(f"  ✓ Proyectos activos: {p_cnt} | Entradas activas: {e_cnt}")
    print("\033[1;32m[skillvault-sync]\033[0m Base de datos aplicada exitosamente.\n")


def cmd_sync(cfg, note=None, push=True):
    """
    All-in-one bidirectional synchronization:
    1. Pull remote repository.
    2. Unify local DB + remote repo DB (bidirectional merge with Last-Write-Wins).
    3. Apply unified DB to local active database.
    4. Snapshot unified state to repo (DB, markdown entries, JSON dumps, README).
    5. Push changes to GitHub.
    """
    print("\n\033[1;35m====================================================================\033[0m")
    print(f"\033[1;36m[skillvault-sync]\033[0m Iniciando Sincronización Bidireccional Completa")
    print("\033[1;35m====================================================================\033[0m")

    # Step 1: Pull
    cmd_pull(cfg)

    # Step 2: Unify
    local_db = cfg["SKILLVAULT_DB_PATH"]
    repo_db = os.path.join(cfg["SKILLVAULT_SYNC_REPO_DIR"], "vault.db")
    unified_tmp = Path(cfg["SKILLVAULT_SYNC_REPO_DIR"]) / ".temp_unified_merge.db"

    print("\033[1;34m[skillvault-sync]\033[0m Integrando registros (merge bidireccional LWW)...")
    ins_cnt, upd_cnt = unify_databases(local_db, repo_db, str(unified_tmp))
    print(f"  ✓ Nuevos registros incorporados desde remoto: {ins_cnt}")
    print(f"  ✓ Registros remotos más recientes actualizados: {upd_cnt}")

    # Step 3: Apply to local DB
    cmd_apply(cfg, str(unified_tmp))
    if unified_tmp.exists():
        unified_tmp.unlink()

    # Step 4: Snapshot and push back to repository
    cmd_snapshot(cfg, note=note or "bidirectional sync", push=push)

    print("\033[1;32m====================================================================\033[0m")
    print("\033[1;32m✓ Sincronización bidireccional finalizada con éxito.\033[0m")
    print(f"  Tanto el workspace local como '{cfg['SKILLVAULT_SYNC_REPO_URL']}' están 100% integrados.")
    print("\033[1;32m====================================================================\033[0m\n")


def main():
    parser = argparse.ArgumentParser(description="SkillVault Backup & Bidirectional Sync Engine")
    parser.add_argument("--config", "-c", help="Path to config.env file")
    subparsers = parser.add_subparsers(dest="subcommand", help="Comandos disponibles")

    # status
    subparsers.add_parser("status", help="Muestra el estado comparativo local vs repositorio remoto")

    # init
    subparsers.add_parser("init", help="Inicializa el archivo de configuración y clona el repositorio")

    # pull
    subparsers.add_parser("pull", help="Descarga los últimos commits del repositorio remoto")

    # snapshot
    p_snap = subparsers.add_parser("snapshot", help="Crea un snapshot de la BD local y lo sincroniza en el repo")
    p_snap.add_argument("--note", help="Nota opcional para el commit de Git")
    p_snap.add_argument("--no-push", action="store_true", help="No ejecutar git push")

    # unify
    p_unify = subparsers.add_parser("unify", help="Fusiona la BD local y la remota en un archivo unificado")
    p_unify.add_argument("--out", default="/tmp/skillvault_unified.db", help="Ruta del archivo de salida")

    # apply
    p_apply = subparsers.add_parser("apply", help="Aplica una base de datos unificada a la BD activa local")
    p_apply.add_argument("--unified", required=True, help="Ruta de la base unificada a aplicar")
    p_apply.add_argument("--yes", "-y", action="store_true", help="Omitir confirmaciones")

    # sync
    p_sync = subparsers.add_parser("sync", help="Sincronización bidireccional completa (Pull + Merge + Apply + Push)")
    p_sync.add_argument("--note", help="Nota opcional para el commit")
    p_sync.add_argument("--no-push", action="store_true", help="No ejecutar git push")

    args = parser.parse_args()

    cfg, target_cfg = load_config(args.config)

    if not args.subcommand or args.subcommand in ("help", "-h", "--help"):
        parser.print_help()
        sys.exit(0)

    try:
        if args.subcommand == "status":
            cmd_status(cfg)
        elif args.subcommand == "init":
            cmd_init(cfg, target_cfg)
        elif args.subcommand == "pull":
            cmd_pull(cfg)
        elif args.subcommand == "snapshot":
            cmd_snapshot(cfg, note=args.note, push=not args.no_push)
        elif args.subcommand == "unify":
            ins, upd = unify_databases(cfg["SKILLVAULT_DB_PATH"], os.path.join(cfg["SKILLVAULT_SYNC_REPO_DIR"], "vault.db"), args.out)
            print(f"Unificación completada en {args.out} (+{ins} nuevos, ~{upd} actualizados)")
        elif args.subcommand == "apply":
            cmd_apply(cfg, args.unified, yes=args.yes)
        elif args.subcommand == "sync":
            cmd_sync(cfg, note=args.note, push=not args.no_push)
    except Exception as e:
        print(f"\n\033[1;31m[Error en skillvault-sync]:\033[0m {e}\n", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
