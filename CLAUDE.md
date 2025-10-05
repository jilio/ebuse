# Claude AI Assistant Guidelines

## Commit Messages

**Use Conventional Commits format:**

```
<type>(<scope>): <subject>

<body>

<footer>
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

**Do NOT add the following to commit messages:**

- "🤖 Generated with [Claude Code](https://claude.com/claude-code)"
- "Co-Authored-By: Claude <noreply@anthropic.com>"

Keep commit messages clean and professional without AI attribution.

## Go Code Style

**Use modern Go syntax:**

- Use `any` instead of `interface{}`
- Use modern type parameters and generics where appropriate
- Follow Go 1.18+ conventions

**Ensure the entire codebase follows these requirements.**
