# Agent Guidance

Read before making changes.

## Principles

- CLEAN code: small functions, single responsibility, descriptive names, no dead code, no overengineering.
- No comments — use descriptive names instead.
- No code markers like `// ... existing code ...` in edits.

## Git

- Do not commit automatically — wait until explicitly asked.
- One commit per concern — never batch unrelated changes.
- Title only, no body. Capitalize first letter after the prefix:
  - `[fix]` bug fix
  - `[feature]` new functionality
  - `[improve]` improvement to existing functionality
  - `[refactor]` formatting, renaming, structural-only
  - `[meta]` deployment, CI
  - `[docs]` documentation
