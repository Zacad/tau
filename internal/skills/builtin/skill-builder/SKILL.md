---
name: skill-builder
description: |
  Create new SKILL.md files following the Agent Skills standard. Use this skill when
  you need to author, update, or validate skill definitions for Tau, PI, OpenCode,
  or Claude Code. Handles frontmatter structure, content organization, and best practices
  for progressive disclosure.
scripts: []
references: []
assets: []
---

# Skill Builder

Create and manage SKILL.md files that conform to the Agent Skills standard (agentskills.io).
Skills are cross-tool compatible — the same SKILL.md works with Tau, PI, OpenCode, and
Claude Code.

## When to Use

- Create a new skill for a capability the agent needs
- Update an existing SKILL.md to follow the latest standard
- Validate skill structure and frontmatter
- Create project-specific skills for workflows or domain knowledge
- Create global skills reusable across projects

## SKILL.md Structure

Every SKILL.md file must have this format:

```markdown
---
name: my-skill-name
description: |
  Clear, concise description of what this skill does and when to use it.
  Keep it under 1024 characters — this is what appears in progressive disclosure.
scripts:
  - scripts/setup.sh        # optional
references:
  - docs/reference.md       # optional
assets:
  - assets/diagram.png      # optional
---

# Skill Name

Detailed capability documentation follows.
```

## Name Rules

- **Characters**: lowercase letters (a-z), digits (0-9), hyphens (-) only
- **Length**: max 64 characters
- **Must start** with a letter or digit
- **Must match** the directory name exactly
- Examples: `code-reviewer`, `security-audit`, `deploy-helper`

## Description Guidelines

The description is critical — it's the only skill content visible in the system prompt
via progressive disclosure. Write it so the agent can decide when to use the skill:

1. **What** the skill does (capability)
2. **When** to use it (trigger conditions)
3. **How** it helps (value)

Good: "Create and manage SKILL.md files following the Agent Skills standard. Use when
authoring new skills, updating existing ones, or validating skill structure."

Bad: "A skill for skills."

## Content Guidelines

The markdown body after frontmatter is loaded only when the agent decides to use the
skill. Structure it for clarity:

1. **Header**: Skill name as H1
2. **When to use**: Clear trigger conditions
3. **How to use**: Step-by-step instructions if applicable
4. **Best practices**: Common patterns and pitfalls
5. **Examples**: Concrete usage examples

## Workflow

### Creating a New Skill

1. Choose a unique, valid name (lowercase, hyphens, max 64 chars)
2. Create directory: `skills/builtin/<name>/` or `~/.tau/skills/<name>/` or `.agents/skills/<name>/`
3. Write SKILL.md with proper YAML frontmatter
4. Include clear description (< 1024 chars) for progressive disclosure
5. Add detailed markdown body with usage instructions
6. Validate: name matches directory, frontmatter is valid YAML, description is meaningful
7. Add references and scripts if needed (paths relative to skill root)

### Updating an Existing Skill

1. Read the current SKILL.md
2. Identify gaps or improvements
3. Update frontmatter and/or content
4. Ensure name still matches directory
5. Validate description length and clarity

### Skill Discovery Tiers

| Tier | Path | Override Priority |
|------|------|-------------------|
| Built-in | Embedded in binary | Lowest |
| Global | `~/.tau/skills/`, `~/.agents/skills/` | Medium |
| Project | `.agents/skills/` (from cwd up) | Highest (overrides) |

## Best Practices

- **One skill per capability**: Don't combine unrelated capabilities
- **Descriptive names**: `security-reviewer` not `sec`
- **Action-oriented descriptions**: Tell the agent WHEN to use this
- **Include examples**: Show, don't just tell
- **Keep references up to date**: Remove stale file references
- **Test before deploying**: Validate with `ParseSkillMD()` or load in Tau
