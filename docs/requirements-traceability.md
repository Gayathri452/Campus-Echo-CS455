# Requirements traceability checklist

This checklist summarizes the functional and non-functional requirements in
`Deliverables/Deliverable_1.pdf`. The PDF is authoritative if this summary and
the deliverable differ.

Status describes the starter currently in this repository; `Not implemented`
means the code should not be presented as satisfying that requirement.

## Functional requirements

| ID | Required behavior | Starter status |
| --- | --- | --- |
| FR-1 | Student sign-up and admin-provisioned moderator accounts issue role-based session tokens. | Implemented for local starter: student signup, administrator CLI bootstrap, administrator-only moderator provisioning, and bearer sessions. College enrollment/email-domain verification is not yet integrated. |
| FR-2 | Login supports Student and Moderator roles; moderator features require moderator authorization. | Partial: all account roles can log in; backend enforces student-only feedback submission and administrator-only moderator provisioning. Moderator queue features are not implemented. |
| FR-3 | Submit a course review with a 1–5 rating and text; persist no student identity and return an anonymous reference code. | Partial: rating, text, anonymous persistence, and reference-code response are implemented; instructor selection and a dedicated asynchronous acceptance response remain different from the full target contract. |
| FR-4 | Show a course's average rating and published reviews only. | Implemented for current course feedback fields: both ratings and exploration filter to published records. |
| FR-5 | Look up review status and flag reason by anonymous reference code. | Implemented through the student-only status endpoint; flag reason is returned when present. |
| FR-6 | Asynchronously classify each review; publish or flag it and record the reason. | Implemented locally with a bounded worker pool, configurable Anthropic provider, deterministic development-only provider, state-checked Mongo updates, and startup/periodic recovery. Live Anthropic behavior requires credentials and has not been certified against the labelled set. |
| FR-7 | Detect near-duplicate reviews for the same course as spam. | Implemented with configurable normalized word-cosine similarity (default 0.82), same-course and chronological checks, and a persisted duplicate reason; this is lexical, not semantic similarity. |
| FR-8 | Flag a course batch for brigading only when velocity is over 15 reviews per 10 minutes **and** average semantic similarity is over 75%; count only reviews longer than five words. | Not implemented |
| FR-9 | Allow one appeal per flagged review, with a short explanation, creating a moderation ticket. | Implemented: a unique appeal reference enforces one ticket per review. |
| FR-10 | Show moderators unclaimed appeal tickets oldest first. | Implemented: only appealed feedback creates a claimable ticket. |
| FR-11 | Atomically claim a ticket so exactly one moderator owns it; concurrent losing claims receive a conflict. | Implemented with an atomic MongoDB claim update. |
| FR-12 | Return a claimed ticket to the queue when released or unresolved after 30 minutes. | Implemented with release and expired-lock handling. |
| FR-13 | Only the owning moderator can resolve a ticket by restoring/publishing or permanently deleting its review. | Implemented with moderator role and active-lock checks. |

The review lifecycle is `Submitted → Published / Flagged → Under Appeal →
Locked → Restored / Deleted`, with a released or expired lock returning to
`Under Appeal`. Preserve these states and ownership constraints in the
implementation.

The proposal's boundary cases are important tests:

- `V = 30`, `S = 30%`: organic post-exam traffic; do not trigger brigading.
- `V < 15`, `S = 95%`: slow duplicate spam; micro-flag duplicates, not a
  brigading batch.
- `V = 20`, `S = 85%`: coordinated attack; trigger a brigading batch flag.

## Non-functional requirements

| ID | Required behavior | Starter status |
| --- | --- | --- |
| NFR-1 | Feedback records contain no student-identifying field; preserve the anonymity boundary. | Partial: user credentials and feedback are in separate MongoDB collections and feedback documents have no user ID. A separate MongoDB database role for triage is not implemented. |
| NFR-2 | The triage agent's database account cannot read the identity collection. | Not implemented |
| NFR-3 | Under 50 simultaneous claims on one ticket, exactly one succeeds in every test run. | Not implemented |
| NFR-4 | Review submission responds within one second independently of AI-service latency. | Submission persists and non-blockingly dispatches; provider latency is outside the handler. The route and browser workflow were verified locally, but a formal under-one-second load benchmark has not been run. |
| NFR-5 | Normal-load triage completes within 60 seconds of submission. | Worker count, queue capacity, request timeout, and recovery interval are configurable; no representative normal-load benchmark has been run. |
| NFR-6 | AI failure/timeout leaves a review `Submitted` and eligible for retry; never publish without triage. | Implemented with bounded exponential retry, expiring Mongo leases, and periodic recovery of still-submitted reviews; failure and retry paths are unit-tested. |
| NFR-7 | On the specified labelled set of 50 reviews, misclassify at most 10% of legitimate criticism. | Not implemented |
| NFR-8 | Hash passwords, enforce role checks in the backend, and use HTTPS for all traffic. | Partial: bcrypt password hashing and backend role checks are implemented. Local HTTP is used for development; production TLS/deployment is not configured. |

## Architecture and project workflow

- Keep the backend a Go monolith with goroutine-based background processing.
- Use MongoDB atomic state-checked operations for contested ticket claims; do
  not substitute an uncoordinated read-then-write or an unnecessary Redis lock.
- Keep the AI gateway/provider configurable for Claude or AWS Bedrock. Never
  expose credentials in source control.
- The current real provider implementation uses the Anthropic Messages API.
  Local development may explicitly select a clearly named mock provider.
- The brigading checker currently uses an explicit development-only no-op
  adapter. The FR-8 algorithm and production integration remain pending; do
  not treat this adapter as a detector.
- Follow the document's staged delivery order: establish authentication,
  submission/viewing/status lookup, appeals, atomic claiming, anonymity, and
  responsiveness before implementing the asynchronous triage and its spam,
  brigading, reliability, and privacy controls.
- Do not add the explicitly rejected chatbot, backend microservices, graph
  database, Redis lock service, or Python/Celery/RabbitMQ processing stack
  without an approved requirements change. The selected approach is a focused
  React/Go/MongoDB application with Go workers.
- The deliverables also call for Jira/GitHub traceability and an initial sprint
  plan. Keep implementation work mapped to the team's Jira issues and PRs.
- Plan the eventual cloud deployment separately (the team has identified AWS or
  GCP as the course deployment target); NFR-8 requires HTTPS.
