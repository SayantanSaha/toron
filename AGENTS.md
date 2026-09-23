# AGENTS.md

## Purpose

This repository uses specialized agents for software engineering.

`AGENTS.md` defines only global policy, ownership, routing, context, and completion rules.

Detailed agent behavior belongs in `.agents/*.md`.
Detailed project knowledge belongs in project artifacts.

---

## Global Rules

1. Read `PRD.md` before project work when it exists.
2. Use the appropriate agent from `.agents/`.
3. Start with minimum required context; expand only when necessary.
4. Never invent requirements, acceptance criteria, API contracts, architecture decisions, evidence, tests, or project conventions.
5. Persistent artifacts are authoritative; handoffs contain only execution/routing state.
6. Preserve artifact ownership and traceability.
7. Use relative links for project documents.
8. Respect required approval gates.
9. Keep changes within approved scope.
10. Distinguish facts, observations, hypotheses, decisions, and unknowns.
11. Record uncertainty rather than presenting assumptions as facts.
12. Do not claim completion when required work remains.
13. Use Graphify/knowledge-graph processing where required by the project workflow.

---

## Agent Registry

| ID | Agent | Specification | Responsibility |
|---|---|---|---|
| AGENT-001 | Project Manager | [.agents/project-manager.md](./.agents/project-manager.md) | Orchestration, routing, approvals |
| AGENT-002 | Requirement Engineer | [.agents/requirement-engineer.md](./.agents/requirement-engineer.md) | Requirements, acceptance criteria |
| AGENT-003 | Development Lead | [.agents/development-lead.md](./.agents/development-lead.md) | Task decomposition |
| AGENT-004 | Architect | [.agents/architect.md](./.agents/architect.md) | Architecture, ADRs |
| AGENT-005 | Test Designer | [.agents/test-designer.md](./.agents/test-designer.md) | Test strategy, test cases |
| AGENT-006 | Developer | [.agents/developer.md](./.agents/developer.md) | Backend/general implementation |
| AGENT-007 | Code Reviewer | [.agents/code-reviewer.md](./.agents/code-reviewer.md) | Code review |
| AGENT-008 | Security Analyst | [.agents/security-analyst.md](./.agents/security-analyst.md) | Security analysis/review |
| AGENT-009 | Document Writer | [.agents/document-writer.md](./.agents/document-writer.md) | Project/release documentation |
| AGENT-010 | System Analyst | [.agents/system-analyst.md](./.agents/system-analyst.md) | Investigation, analysis, root cause, impact |
| AGENT-011 | Frontend Developer | [.agents/frontend-developer.md](./.agents/frontend-developer.md) | Frontend implementation and maintenance |

Agent instructions:

`.agents/<agent-name>.md`

---

## Artifact Ownership

| Artifact | Owner |
|---|---|
| REQ | Requirement Engineer |
| AN | System Analyst |
| TASK | Development Lead |
| ADR | Architect |
| TC | Test Designer |
| Source/Test Code | Developer / Frontend Developer |
| CR | Code Reviewer |
| SR | Security Analyst |
| WIKI / Release Docs | Document Writer |

Substantial System Analyst work should normally be persisted as:

`docs/analysis/AN-XXX.md`

Simple analysis may remain inline when no durable artifact is required.

---

## Routing

The Project Manager owns workflow routing.

### Primary routing

```text
Investigation / unexplained problem
    → AGENT-010 System Analyst

Requirements / scope
    → AGENT-002 Requirement Engineer

Task decomposition
    → AGENT-003 Development Lead

Architecture / technical decision
    → AGENT-004 Architect

Test strategy / test cases
    → AGENT-005 Test Designer

Backend / general implementation
    → AGENT-006 Developer

Frontend implementation
    → AGENT-011 Frontend Developer

Code review
    → AGENT-007 Code Reviewer

Security review
    → AGENT-008 Security Analyst

Documentation
    → AGENT-009 Document Writer
```

### System Analysis Gate

Use **AGENT-010 System Analyst** when the request requires investigation or analysis, including:

- bugs or unexplained failures,
- ambiguous/intermittent behavior,
- observations requiring investigation,
- root-cause analysis,
- cross-component impact analysis,
- feature/change impact analysis,
- performance/reliability problems,
- architecture/integration questions,
- configuration/environment differences,
- incidents or operational problems,
- determining what is happening or what should change.

System Analyst does not replace Requirement Engineer, Architect, Test Designer, Developer, Frontend Developer, or Security Analyst.

If scope and desired behavior are already clear, PM may route directly to Requirement Engineer or the appropriate specialist.

### Cross-domain work

For changes spanning frontend and backend:

```text
Frontend → AGENT-011
Backend  → AGENT-006
```

Invoke both when required.

Do not invent or alter API contracts outside the appropriate ownership.

---

## Review Routing

Run Code Reviewer and Security Analyst in parallel when both apply.

Detailed findings remain in:

```text
CR-XXX.md
SR-XXX.md
```

`review_result` is only a routing signal. It must not replace the detailed review artifact.

Route findings according to resolution type:

```text
implementation → Developer / Frontend Developer
test           → Test Designer and/or implementation agent
architecture   → Architect → implementation
requirement    → Requirement Engineer
analysis       → System Analyst
security       → Security Analyst / implementation agent
documentation  → Document Writer
accepted_risk  → record decision → continue
```

---

## Context Policy

Use progressive disclosure.

### Start with

```text
PRD.md
user request
assigned artifact/task
directly related requirements
directly relevant source/configuration
```

### Expand only when required by

```text
ambiguity
dependency
architecture impact
security risk
regression risk
missing API contract
missing evidence
review finding
test dependency
```

Do not load the entire repository or project documentation by default.

---

## Handoff Protocol

Use:

`.agents/agent-protocol.md`

A handoff should contain only what the next agent needs to continue:

```text
status
artifact IDs/paths
blockers
required action
next agent
```

Do not copy large artifact contents into handoffs.

### Artifact vs Handoff

```text
Artifact  = durable project knowledge/evidence
Handoff   = minimal execution/routing state
```

### Analysis

When persisted:

```text
AN-XXX.md = complete analysis
result     = routing/status summary
```

### Reviews

```text
CR-XXX.md = complete code-review findings
SR-XXX.md = complete security findings
review_result = routing/status summary
```

---

## Approval and Ownership

- Requirement Engineer owns formal requirements and acceptance criteria.
- Architect owns architecture decisions and ADRs.
- Test Designer owns test-case design.
- Developer/Frontend Developer own implementation within their domains.
- Code Reviewer owns code-review findings.
- Security Analyst owns security-review findings.
- Document Writer owns project documentation.
- System Analyst owns persistent system-analysis artifacts.
- Project Manager owns orchestration and routing.

An agent may identify or propose a change to another domain but must not silently make that domain's authoritative decision.

---

## Scope Discipline

Agents must not:

- invent scope,
- perform unrelated refactoring,
- bypass approval gates,
- silently rewrite another agent's authoritative artifact,
- introduce unnecessary dependencies,
- hide failed or skipped checks,
- claim unverified behavior,
- convert hypotheses into facts.

Prefer the smallest coherent change satisfying the approved scope.

---

## Completion Criteria

Before declaring work complete:

```text
✓ Acceptance criteria addressed
✓ Required implementation completed
✓ Required tests/checks executed or explicitly reported as not run
✓ Review findings resolved, accepted, or explicitly left open
✓ Security implications addressed where applicable
✓ Required documentation updated
✓ Traceability preserved
✓ No known required action remains unreported
```

---

## Default Lifecycle

The normal lifecycle is:

```text
User
  ↓
System Analyst*
  ↓
Requirement Engineer
  ↓
Approval
  ↓
Development Lead
  ↓
Architect
  ↓
Test Designer
  ↓
Developer / Frontend Developer
  ↓
Code Reviewer + Security Analyst*
  ↓
Review Routing*
  ↓
Document Writer*
  ↓
Regression Test + Graphify
  ↓
Commit
```

`*` Conditional.

The Project Manager may skip any agent whose responsibility is demonstrably irrelevant.

---

## Final Rule

**Project Manager orchestrates.  
Specialists execute.  
Artifacts preserve knowledge.  
Handoffs preserve efficiency.**
