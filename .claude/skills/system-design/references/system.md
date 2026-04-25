# System Design Reference

Detailed workflow for system, API, component, and data model design. Load this file when executing the system-design skill workflow.

## 1. Gather Requirements and Constraints

Before any design work, capture:

- **Functional requirements**: What the system must do (user-facing capabilities, business rules).
- **Non-functional requirements**: Performance, availability, scalability, security, compliance targets.
- **Constraints**: Platform, language, existing systems, team skills, deadlines, budget.
- **Assumptions**: Explicitly state what you're assuming; flag unknowns for follow-up.
- **Stakeholders**: Who consumes the design (engineers, SRE, product, security, auditors).

Output of this step: a short requirements brief — bulleted, not prose.

## 2. Define Structure, Interfaces, and Data Flows

### Structure
- Identify bounded contexts or modules.
- Name each component with a single responsibility.
- Draw component boundaries; explicitly mark what's in-scope vs out-of-scope.

### Interfaces
- For each component boundary: define inputs, outputs, errors, and ownership.
- For APIs: specify endpoints, methods, request/response schemas, auth, versioning, rate limits.
- For data models: specify entities, fields (with types + nullability), relationships, indexes, and invariants.

### Data Flows
- Trace the critical paths: request lifecycle, data ingestion, event propagation.
- Identify synchronous vs asynchronous hops.
- Call out failure modes at each hop (timeout, retry, fallback, dead-letter).

## 3. Validate Against Constraints and Best Practices

Run the design through a checklist:

- **Correctness**: Does the design satisfy every functional requirement?
- **Scale**: Does it meet performance/availability targets at expected load (and 10x)?
- **Failure modes**: What happens when each dependency fails? Is data loss possible?
- **Security**: AuthN/AuthZ at every boundary. Sensitive data minimized, encrypted in transit + at rest.
- **Observability**: Are the right logs, metrics, and traces emitted to debug production issues?
- **Evolvability**: Can this be extended without breaking consumers? Versioning strategy defined?
- **Cost**: Rough cost envelope — infra, operational, maintenance.
- **Complexity**: Is there a simpler design that meets the requirements? Justify added complexity.

## 4. Deliver Artifacts

Choose the right artifact for the audience:

- **Design spec (Markdown)**: For engineers implementing or reviewing.
  - Sections: Context, Requirements, Design, Alternatives Considered, Open Questions, Rollout.
- **Diagrams**: For visual communication of architecture and flows.
  - C4 model (Context, Container, Component, Code), sequence diagrams, data flow diagrams.
- **API specifications**: OpenAPI/Protobuf schema for APIs.
- **Data model docs**: ERDs, DDL, or schema files with comments on invariants.
- **ADR**: Architecture Decision Record for significant irreversible decisions.

Every artifact should include:
- **Validation notes**: What was checked, what passed, what's deferred.
- **Follow-ups**: Open questions, risks, and owners.

## Common Pitfalls

- **Designing without constraints**: Leads to over- or under-engineered solutions. Always anchor to requirements.
- **Mixing implementation with spec**: Keep "what" and "why" in the design; push "how" to implementation tickets.
- **Skipping alternatives**: Document at least one alternative and why it was rejected.
- **Ignoring failure modes**: A design that only covers the happy path is incomplete.
- **No rollout/migration plan**: Designs for existing systems must address how to get from current to target state.
