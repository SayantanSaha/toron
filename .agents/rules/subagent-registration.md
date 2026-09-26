---
trigger: always_on
description: Define sub-agents when required from the agent definition
---

## Subagent Registration Policy
    When delegating to any agent listed in the Agent Registry:
    1. Check if the subagent type is already registered.
    2. If not registered, the Project Manager must automatically call `define_subagent`
       using the role, description, and system prompt specified in `.agents/<agent-name>.md`.
    3. Dispatch the subagent via `invoke_subagent`.