---
name: Publicar nuevo Skill / Workflow
about: Proponer la adición o publicación de un nuevo Agent Skill o Workflow
title: 'skill: [nombre del skill o workflow]'
labels: ['type:skill']
assignees: ''
---

## Información del Skill

- **Nombre Identificador:** (ejemplo: `q:mi-skill`)
- **Triggers:** (ejemplo: `/mi-skill`, palabras clave de activación)
- **Taxonomía LifeOS sugerida:** (`WORK` | `KNOWLEDGE` | `LEARNING` | `RELATIONSHIP` | `STATE` | `OBSERVABILITY`)
- **Modelos / Agentes Objetivo:** (Claude Code, Cursor, Windsurf, Antigravity, OpenCode)

## Propósito y Funcionalidad
Explica qué hace este skill y por qué es útil para los agentes de IA.

## Contenido Preliminar de `SKILL.md`

```markdown
---
name: q:mi-skill
description: "Trigger: /mi-skill..."
version: 1.0.0
license: MIT
---

# Instrucciones del Skill...
```

## Checklist de Calidad
- [ ] No contiene rutas locales absolutas personales (ej. `/home/...`)
- [ ] No contiene tokens, claves API ni contraseñas
- [ ] Todos los bloques de código tienen etiquetas de lenguaje explícitas
- [ ] Validado con `npx @skills-hub-ai/cli lint` o `npx skills`
