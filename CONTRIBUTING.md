# Toron Coding Standards

This document outlines the coding standards for the **Toron** project to ensure consistency, readability, and maintainability of the codebase.

---

## 1. Code Style

- Use `gofmt` or `goimports` for formatting. No code should be merged without being formatted.
- Follow idiomatic Go principles outlined in [Effective Go](https://golang.org/doc/effective_go.html).
- Use `golangci-lint` with essential linters:
  - `errcheck`
  - `staticcheck`
  - `govet`
  - `unused`
  - `gosimple`

---

## 2. Project Structure

```
cmd/toron           - Main application entry point
internal/           - Private, non-reusable internal modules
pkg/                - Public packages for reuse
plugins/            - Built-in and external plugin modules
config/             - Configuration schemas and parsing
api/                - Admin and public APIs
middleware/         - HTTP middleware chain components
docs/               - Markdown documentation and assets
```

---

## 3. Naming Conventions

- Use camelCase for variable names and functions: `handleRequest`, `userID`
- Use PascalCase for exported types and functions: `ConfigLoader`, `NewRouter`
- Use ALL\_CAPS only for constants meant to be global or externally visible (e.g., `DEFAULT_PORT`).
- File names should be lowercase, hyphenated where necessary: `http-server.go`, `rate-limiter.go`

---

## 4. Best Practices

- Avoid global state. Use dependency injection where appropriate.
- Use `context.Context` for timeouts, cancellations, and logging context.
- Functions should do one thing only and be testable.
- Avoid premature optimization. Focus on clarity first.
- Use slices over arrays. Use maps when lookups are needed.
- Use interfaces only when needed — prefer concrete types internally.

---

## 5. Error Handling

- Always check for errors.
- Wrap errors with context using `fmt.Errorf("description: %w", err)` or a logger.
- Avoid panic unless absolutely necessary (e.g., in `main()` or test setup).
- Use sentinel errors (`var ErrFoo = errors.New("foo")`) when needed.

---

## 6. Documentation

- All exported functions, types, and packages must have GoDoc comments.
- Each major package should include a `doc.go` file.
- Internal logic should be documented inline where helpful.
- Add `README.md` to top-level and major submodules to explain purpose and usage.

---

## 7. Testing Standards

- Use Go's built-in `testing` package.
- Each package should include unit tests with table-driven style.
- Test file naming: `xyz_test.go`
- Use `testify` for assertions if needed.
- Coverage should exceed 85% for core packages.
- Add benchmarks for critical code paths.

---

## 8. Versioning & Release

- Follow Semantic Versioning: `MAJOR.MINOR.PATCH`
- Tag releases in Git with appropriate changelogs.
- All releases should include binary, Docker image, and changelog.

---

## 9. Security Practices

- Validate all config and inputs explicitly.
- Deny by default in routers and middleware.
- Avoid string concatenation in SQL-like logic.
- All cryptography must use Go standard libraries or audited packages.

---

## 10. CI/CD Expectations

- All commits are linted and tested via GitHub Actions.
- No commit is accepted into `main` without a passing test suite.
- PRs require review and must be squash-merged.

---

## 11. Code Review Guidelines

- Focus on correctness, readability, and testability.
- Suggest alternatives constructively.
- Block PRs with failing tests or undocumented exports.

---

## 12. External Dependencies

- Must be stable and licensed permissively (MIT, BSD, Apache 2.0).
- Version-pinned in `go.mod`; no indirect dependencies without reason.

---

**Toron** — Engineered with clarity, built for scale.
