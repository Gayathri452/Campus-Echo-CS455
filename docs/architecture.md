# CampusEcho architecture and implementation status

## Target architecture from the deliverables

CampusEcho is a modular Go monolith, not a set of backend microservices:

- **Presentation:** React student, moderator, and administrator dashboards.
- **Application:** Go REST API, authentication/role checks, feedback and ticket
  workflows, plus a goroutine-based asynchronous AI triage worker pool.
- **Data:** MongoDB collections for credentials, anonymous feedback payloads,
  and moderation tickets. The triage agent must not be able to read identity
  data.
- **AI:** Claude or AWS Bedrock behind an AI gateway; AI calls must not block
  feedback submission.

MongoDB atomic updates are required for moderation-ticket claims. The design
rejects a separate Redis locking dependency in favor of MongoDB state-checked
atomic updates.

## Current vertical slice

The React student feedback page submits to the Go API through Vite's `/api`
development proxy. The API validates the request and inserts an anonymous
feedback document into MongoDB as `flagged` while AI moderation is unavailable.
Flagged submissions stay out of the moderator queue until a student submits
their single appeal. That appeal creates an `under_appeal` ticket in the
moderator collection; moderator resolution updates both the ticket and review.

```text
StudentFeedbackPage
  -> frontend/src/services/feedback.ts
  -> POST /api/v1/feedback
  -> backend/internal/httpapi
  -> backend/internal/feedback validation
  -> backend/internal/database MongoDB store
```

The feedback record contains a MongoDB ID, reference code, course details,
content, status, flag reason, and creation timestamp. It does not contain a
student identity. AI classification is not connected yet, so the current
development behavior flags every submission with a fixed explanatory reason.
Students can look up that state by reference code, appeal once, and see the
appeal status and moderator resolution note.

The starter authentication adds separate `users` and `sessions` collections.
Passwords are bcrypt-hashed; clients receive random 12-hour bearer tokens while
MongoDB stores only their SHA-256 hashes. Student registration assigns the
student role server-side. An administrator is created by an interactive
one-time CLI command and can provision moderator accounts; role checks are
enforced by the API. The React client holds its bearer token in memory rather
than browser persistent storage.

The current setup does not verify a college email against an institution, does
not yet expose moderator queue operations, and has no independent MongoDB
database account for the future triage worker. Production HTTPS must be
provided by the deployment environment.

## Implementation boundary

Use [requirements-traceability.md](./requirements-traceability.md) as the
checklist before treating any feature as complete. Do not silently replace
specified behavior—especially the brigading thresholds, anonymous reference
code workflow, status transitions, ticket ownership, or timeout—with a simpler
alternative.

Keep package-specific Go tests beside their code. Use `tests/integration/` for
tests that exercise multiple services or a real MongoDB instance.
