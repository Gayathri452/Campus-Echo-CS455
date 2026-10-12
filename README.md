# CampusEcho

CampusEcho is a privacy-preserving course and faculty feedback system. Students
can submit feedback anonymously; the planned system also includes automated
moderation and a human appeal workflow.

## Stack

- Frontend: React, TypeScript, Vite, and Tailwind CSS
- Backend: Go HTTP API
- Database: MongoDB
- Planned AI integration: Claude / AWS Bedrock

## Current starter

This repository is a bootstrap, not an implementation of all requirements in
the deliverables. Its current vertical slice accepts a course ID and feedback
text from an authenticated student and stores them in MongoDB with a
`submitted` status. Student login, administrator bootstrap, and moderator
provisioning are implemented, but student enrollment is not verified against
an institution. Feedback still lacks instructor selection, a 1–5 rating, and
an anonymous reference code; background triage and the review/moderation
workflows are also not implemented. The full gap list is in
[the requirements checklist](./docs/requirements-traceability.md).

## Run locally

Prerequisites: Go 1.22+, Node.js 18+, npm, and a MongoDB server.

1. Install MongoDB Community Edition using the
   [official installation guide](https://www.mongodb.com/docs/manual/administration/install-community/),
   then start the MongoDB service. The default local connection is
   `mongodb://localhost:27017`.

2. Create the first administrator account once, from an interactive terminal:

   ```powershell
   cd backend
   go run ./cmd/create-admin
   ```

   Enter the administrator email and password when prompted; password entry is
   hidden. The password must be at least 6 characters. This command only
   creates an administrator and does not reset or reveal existing passwords.

3. In one terminal, start the API from the repository root:

   ```sh
   cd backend
   go run ./cmd/api
   ```

   The API listens on `http://localhost:8080`. It reads configuration from
   process environment variables; `.env.example` documents the variable names
   but the Go process does not load a `.env` file automatically.

4. In another terminal, start the frontend from the repository root:

   ```powershell
   cd frontend
   npm.cmd install
   npm.cmd run dev
   ```

   `npm.cmd` avoids PowerShell's script-execution restriction on `npm.ps1`.
   In macOS/Linux shells, use `npm install` and `npm run dev` instead. Open the
   local URL printed by Vite. The development server proxies `/api` requests to
   the Go API.

## Current starter API

- `GET /api/v1/health` — liveness check
- `POST /api/auth/request-otp` — validate an IITK email and password registration request, then send an OTP
- `POST /api/auth/verify-otp` — verify the OTP, create the student account, and establish a session
- `POST /api/auth/student/login` — student-only password login
- `POST /api/auth/admin/login` — administrator-only password login
- `POST /api/auth/moderator/login` — moderator-only password login
- `POST /api/v1/auth/login` — log in; returns a 12-hour bearer session
- `POST /api/v1/auth/logout` — revoke the bearer session
- `GET /api/v1/auth/me` — return the signed-in account
- `POST /api/v1/admin/moderators` — administrator-only moderator provisioning
- `DELETE /api/v1/admin/moderators/{email}` — administrator-only moderator removal
- `POST /api/v1/feedback` — student-only anonymous review submission
- `GET /api/v1/feedback/explore` — student-only published review search and filters
- `GET /api/v1/feedback/ratings` — student-only published course rating summaries
- `GET /api/v1/feedback/status/{reference}` — student-only reference status lookup
- `POST /api/v1/feedback/status/{reference}/appeal` — appeal a flagged or rejected review

Review submissions accept:

  ```json
  {
    "courseId": "CS455",
      "courseTitle": "Software Engineering",
      "category": "teaching",
      "rating": 4,
    "content": "The weekly examples helped me understand the material."
  }
  ```

Authenticated endpoints use `Authorization: Bearer <accessToken>`. The React
starter keeps the token in memory; reloading the page signs the user out. User
credentials and sessions are stored separately from feedback, and passwords
are bcrypt-hashed. The initial administrator is created with the interactive
command above. Student registration currently checks email syntax only; it
does not verify college enrollment or a college email domain. Add that
institutional verification before relying on registration outside local
development.

The current feedback endpoint responds with HTTP 201 and a database ID. This is
deliberately only a development slice: the requirements document calls for an
anonymous reference code and asynchronous acceptance/triage, among other
differences. Treat the deliverables—not this starter endpoint—as the target
contract.

The local Go server uses HTTP. Do not deploy it publicly without HTTPS; use a
TLS-terminating reverse proxy or the approved cloud platform's HTTPS service.

## Tests

From the repository root, run backend unit tests with:

```powershell
Set-Location backend
go test ./...
```

Frontend scripts are defined in `frontend/package.json`. Integration-test setup
is documented in [tests/integration/README.md](./tests/integration/README.md).