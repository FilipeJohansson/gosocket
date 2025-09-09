# Contributing to GoSocket

Thanks for your interest in contributing to **GoSocket**!  
This document explains how to contribute, the rules we follow, and the expectations for contributors.

---

## Workflow

- We use a **single `main` branch** as the source of truth.  
- New work should be developed in a dedicated branch following this pattern:
  - feature/my-feature
  - fix/my-bug
  - docs/my-docs-update
- Open a Pull Request (PR) from your branch into `main`.

---

## Commit Messages

We follow the [Conventional Commits](https://www.conventionalcommits.org/) convention:

- `feat:` for new features  
- `fix:` for bug fixes  
- `docs:` for documentation changes  
- `test:` for tests only  
- `chore:` for maintenance (build, CI, dependencies, etc.)  

**Example:**

feat: add support for custom WebSocket handlers

---

## Code Style

- All code must be formatted with:
  ```bash
  gofmt -s -w .
  go vet ./...
  golangci-lint run
  ```
- Keep code idiomatic and clean.

---

## Tests

- All contributions must include tests when applicable.
- Before opening a PR, run:
    ```bash
    go test ./...
    ```
- Contributions must keep minimum 70% test coverage.

---

## Opening Issues

Please use the provided [Issue Template](./.github/ISSUE_TEMPLATE/issue_template.md).

For bugs, include:
- GoSocket version
- Go version
- Operating system
- Steps to reproduce
- Expected vs. actual result

For features, include:
- Description of the functionality
- Motivation (why it would be useful)
- Alternatives considered

---

## Contributions Accepted

We welcome contributions in:
- Code (features, bugfixes, refactors)
- Documentation
- Examples
- Tests

---

## Code of Conduct

By participating, you agree with our [Code of Conduct](./CODE_OF_CONDUCT.md).

---

Happy coding!
