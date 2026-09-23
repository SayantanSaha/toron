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

## Execution Principle

**Artifact = detailed persistent knowledge.  
Handoff = minimal execution state.**

Agents should communicate using `.agents/agent-protocol.md`.
