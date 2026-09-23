# Migration Guide

## What changed

The optimized system separates:

1. Global rules
2. Agent-specific behavior
3. Artifact schemas
4. Agent-to-agent handoffs
5. Review evidence
6. Review routing state

## Recommended project layout

```text
AGENTS.md
.agents/
  agent-protocol.md
  project-manager.md
  requirement-engineer.md
  development-lead.md
  architect.md
  test-designer.md
  developer.md
  code-reviewer.md
  security-analyst.md
  document-writer.md
  schemas/
    requirement.md
    task.md
    adr.md
    test-case.md
    review.md
```

## Key behavioral changes

### 1. Targeted context

Agents no longer need the full project document set by default. Pass artifact IDs and retrieve only relevant sections.

### 2. Detailed reviews remain

CR/SR artifacts still contain complete findings, evidence, impact, recommendations, test gaps, and residual risk.

### 3. Compact review result

CR/SR additionally emit a machine-readable routing summary. The Project Manager uses this summary and does not need to parse the full review to determine the next agent.

### 4. Review routing

Do not automatically loop through Requirement Engineer after every review finding.

- implementation/test -> Developer
- architecture -> Architect -> Developer
- requirement -> Requirement Engineer -> approval if scope changes
- documentation -> Document Writer
- accepted risk -> record and continue

### 5. Conditional execution

Skip Architect, Security, Document Writer, or other agents when their work is demonstrably unnecessary for the change.

### 6. Artifact authority

The persistent artifact is authoritative. Handoffs must reference IDs rather than copy artifact contents.

## Migration strategy

1. Replace the existing agent files with the optimized definitions.
2. Add the protocol and schemas.
3. Update the PM orchestration implementation to use the compact handoff/result protocol.
4. Keep existing project artifacts; their content does not need to be rewritten solely for this optimization.
5. On the first workflow using the new system, verify that each agent can locate the referenced artifact by ID/path.
6. Measure prompt/context tokens before and after several representative tasks.

## Expected effect

The static agent instructions are substantially smaller, while detailed review and engineering artifacts remain intact. The largest runtime savings should come from targeted context and routing review findings directly to the responsible agent.
