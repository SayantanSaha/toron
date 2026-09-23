# Agent System Rules

## Global Rules

1. Read `PRD.md` and only the project artifacts relevant to the current task.
2. Use the agents defined in `.agents/` for software changes.
3. Preserve artifact ownership and traceability.
4. Never invent requirements, acceptance criteria, architecture decisions, or expected behavior.
5. Use relative links only. Never use absolute filesystem paths in project documentation.
6. Reference existing artifacts by ID/path; do not reproduce their contents in agent handoffs.
7. Persistent artifacts are the source of truth. Agent handoffs are compact execution state.
8. Start with minimal context and retrieve additional context only when ambiguity or risk requires it.
9. Do not modify artifacts owned by another agent unless explicitly authorized by the workflow.
10. Keep user-facing output concise; return detailed reasoning in the artifact when the task requires a persistent record.
11. For questions requiring project knowledge, use the project knowledge graph where appropriate.
12. Never mark an artifact approved unless the workflow explicitly permits it and the required approval exists.

## Artifact Ownership

| Artifact | Owner |
|---|---|
| REQ | Requirement Engineer |
| TASK | Development Lead |
| ADR | Architect |
| TC | Test Designer |
| Source/Test Code | Developer |
| CR | Code Reviewer |
| SR | Security Analyst |
| WIKI | Document Writer |

## Agents Directory

| ID | Agent | Specification | Primary Artifact / Output |
|---|---|---|---|
| AGENT-001 | Project Manager | [.agents/project-manager.md](./.agents/project-manager.md) | Pipeline coordination, verification & commit |
| AGENT-002 | Requirement Engineer | [.agents/requirement-engineer.md](./.agents/requirement-engineer.md) | `docs/requirements/REQ-XXX.md` |
| AGENT-003 | Development Lead | [.agents/development-lead.md](./.agents/development-lead.md) | `docs/tasks/TASK-XXX.md` |
| AGENT-004 | Architect | [.agents/architect.md](./.agents/architect.md) | `docs/architecture/ADR-XXX.md` |
| AGENT-005 | Test Designer | [.agents/test-designer.md](./.agents/test-designer.md) | `docs/testCases/TC-XXX.md` |
| AGENT-006 | Developer | [.agents/developer.md](./.agents/developer.md) | Source code & unit/integration tests |
| AGENT-007 | Security Analyst | [.agents/security-analyst.md](./.agents/security-analyst.md) | `docs/securityReview/SR-XXX.md` |
| AGENT-008 | Code Reviewer | [.agents/code-reviewer.md](./.agents/code-reviewer.md) | `docs/codeReview/CR-XXX.md` |
| AGENT-009 | Document Writer | [.agents/document-writer.md](./.agents/document-writer.md) | `docs/wiki/*` |

## Execution Principle

**Artifact = detailed persistent knowledge.  
Handoff = minimal execution state.**

Agents should communicate using [.agents/agent-protocol.md](./.agents/agent-protocol.md).
