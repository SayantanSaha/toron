---
trigger: always_on
description: Enforce strictly relative links and forbid absolute file paths in all documentation across all agents.
---

## Strictly Relative Links (No Absolute Paths)

All agents creating or updating any sort of document (requirements, tasks, architecture records, test cases, code reviews, security reviews, implementation notes, user guides, or wiki pages) must strictly follow these rules:

1. **NEVER Use Absolute File Paths**: Never use absolute file paths (such as `file:///...`, `/Users/...`, leading slash filesystem paths, or absolute machine URIs) in any document.
2. **Strictly Relative Links**: All links between documents or to files must be relative links (e.g., `../requirements/REQ-001.md`, `./configuration.md`).
3. **WIKI Isolation**: In WIKI documentation (`docs/wiki/`), never refer to, link to, or mention any documentation outside of `wiki/`. Source code may be referenced as inline code backticks (e.g., `pkg/router/router.go`), but never with absolute paths or external hyperlinks.
