# Review Artifact Schema

Use the same structure for Code Review (CR) and Security Review (SR).

```yaml
id: CR-XXX | SR-XXX
type: code-review | security-review
title: ...
status: draft | approved | changes_required
version: 1
project: PROJECT-001
owner: code-reviewer | security-analyst
derived_from: [SOURCE-CODE]
depends_on: [TASK-XXX]
verifies: [REQ-XXX, TASK-XXX, TC-XXX]
decided_by: [ADR-XXX]
related_to: []
```

## Review Summary
```yaml
status: approved | changes_required | blocked
blocking: true | false
findings:
  critical: 0
  high: 0
  medium: 0
  low: 0
  informational: 0
required_changes: []
resolution: {}
```

## Scope
- Files reviewed:
- Artifacts reviewed:

## Findings

### Finding ID: Title
- Severity:
- Category:
- Location:
- Related artifacts:
- Evidence:
- Description:
- Impact:
- Recommendation:
- Required action:

## Test Coverage Review / Missing Security Tests
...

## Positive Observations
...

## Residual Risk
...

## Open Questions
...
