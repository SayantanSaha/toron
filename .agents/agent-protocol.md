# Agent Protocol

Use this compact protocol for agent-to-agent handoffs. Do not copy artifact contents into the handoff.

## Handoff

```yaml
handoff:
  from: AGENT-XXX
  to: AGENT-XXX
  action: <action>

  primary:
    - <artifact-id>

  context:
    requirements: []
    tasks: []
    architecture: []
    tests: []
    reviews: []
    analysis: []

  scope:
    include: []
    exclude: []

  constraints: []
  questions: []

  expected:
    artifacts: []
```

Only populate fields that matter.

## Agent Routing Vocabulary

```text
ANALYSIS      = system-level investigation before implementation
FRONTEND      = frontend implementation/maintenance
BACKEND       = backend implementation
ARCHITECTURE  = architecture/design decision
REQUIREMENT   = requirement or acceptance-criteria change
TEST          = test design/coverage work
SECURITY      = security-specific investigation/remediation
DOCUMENTATION = user-facing documentation
```

## Result

```yaml
result:
  agent: AGENT-XXX
  status: completed | changes_required | blocked | needs_user

  artifacts:
    - <path-or-id>

  decisions: []
  findings: []
  blockers: []

  next:
    agent: AGENT-XXX
    action: <action>
```

## Review Result

Review agents additionally return:

```yaml
review_result:
  status: approved | changes_required | blocked
  blocking: true | false

  findings:
    critical: 0
    high: 0
    medium: 0
    low: 0
    informational: 0

  required_changes:
    - <finding-id>

  resolution:
    <finding-id>: implementation | architecture | requirement | test | documentation | analysis | accepted_risk
```

The full review remains in the CR/SR artifact. The result only routes workflow.

## Context Policy

1. Start with IDs and relevant sections.
2. Read complete artifacts only when necessary.
3. Prefer changed files over the whole repository.
4. Never repeat source artifact text in a handoff.
