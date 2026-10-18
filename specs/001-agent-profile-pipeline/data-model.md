# Data Model: Agent Profile Pipeline

## Job (extended)

- `profile_outputs TEXT NOT NULL DEFAULT '{}'` — JSON map of profile name → agent output text.

## ProfileOutputs

- Map[string]string serialized as JSON. Keys: spec, plan, task, analyze, implement, converge, reviewer.

## Agent.Profile

- Typed string constants for the seven profiles. `ProfileOrder` defines execution order.

## Storage operations

- `SetJobProfileOutputs(db, id, outputs string) error` — persists JSON.