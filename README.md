# CampusEcho

CampusEcho is a privacy-preserving course and faculty feedback system. Students
can submit feedback anonymously; the planned system also includes automated
moderation and a human appeal workflow.

## Stack

- Frontend: React, TypeScript, Vite, and Tailwind CSS
- Backend: Go HTTP API
- Database: MongoDB
- AI triage providers: Google Gemini API, Anthropic Claude Messages API, or an
  explicit development mock; configurable by environment

## Current starter

This repository is a bootstrap, not an implementation of all requirements in
the deliverables. Its current vertical slice accepts a course ID and feedback
text from an authenticated student and stores it anonymously in MongoDB with
the `submitted` status before asynchronous triage. Student login,
administrator bootstrap, and moderator provisioning are implemented, but
student enrollment is not verified against an institution. The full gap list
is in
[the requirements checklist](./docs/requirements-traceability.md).

## Run locally

Prerequisites: Go 1.24+, Node.js 18+, npm, and a MongoDB server.

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

3. Copy the example configuration once. For Gemini, edit the ignored local
   `.env` file and set `TRIAGE_PROVIDER=gemini`, paste your key after
   `GEMINI_API_KEY=`, and keep `GEMINI_MODEL=gemini-2.5-flash-lite`:

   ```powershell
   Copy-Item .env.example .env
   notepad .env
   git check-ignore .env
   ```

   `git check-ignore` should print `.env`. Get a Gemini API key from
   [Google AI Studio](https://aistudio.google.com/app/apikey). Keep the key in
   this local backend `.env` only; do not add it to frontend variables, source
   files, screenshots, or commits. `.env` is ignored by Git; `.env.example`
   contains placeholders only.

   Start the API in one terminal:

   ```powershell
   Set-Location backend
   $env:TRIAGE_PROVIDER = "gemini"
   go run ./cmd/api
   ```

   The API listens on `http://localhost:8080`. Go loads `.env` from the current
   directory or its parent directory. The example uses console email mode, so
   local student-registration OTP codes appear in the API terminal. Successful
   configuration prints the selected Gemini model and confirms that the key is
   present, without printing the key itself. Startup fails clearly if Gemini is
   selected with an empty key; a bad key or unavailable model is reported when
   the first review is classified.

4. In another terminal, start the frontend from the repository root:

   ```powershell
   cd frontend
   npm.cmd install
   $env:CAMPUSECHO_API_TARGET = "http://localhost:8080"
   npm.cmd run dev
   ```

   `npm.cmd` avoids PowerShell's script-execution restriction on `npm.ps1`.
   In macOS/Linux shells, use `npm install` and `npm run dev` instead. Open the
   local URL printed by Vite. The development server proxies `/api` requests to
   the Go API; set `CAMPUSECHO_API_TARGET` when using a non-default backend
   port.

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

### Local AI triage configuration

The example environment explicitly selects `TRIAGE_PROVIDER=development-mock`
so the end-to-end workflow can be exercised without sending review text to an
external service. This deterministic development classifier is **not real AI**
and is rejected when `APP_ENV` is not `development`.

To use Gemini, set `TRIAGE_PROVIDER=gemini` and provide `GEMINI_API_KEY` in the
ignored local `.env` or the backend process environment. `GEMINI_MODEL` defaults
to `gemini-2.5-flash-lite`, which Google's current model guide lists as suited
to high-volume classification. Gemini models and free-tier eligibility can
change; check Google's [model list](https://ai.google.dev/gemini-api/docs/models),
[pricing](https://ai.google.dev/gemini-api/docs/pricing), and the model's
availability in your Google AI Studio project before use. The free tier has
model-specific rate limits and is limited to eligible models. Do not enable
billing if you want to avoid paid usage. Google may use Gemini Free Tier
requests to improve its products; review Google's current
[API terms](https://ai.google.dev/gemini-api/terms) before sending real student
feedback. Only review text is sent to Gemini;
student identity, authentication data, and ratings are not sent. The backend
requests JSON-schema output and validates the decision, reason code, and
explanation. Strong criticism, negative sentiment, and low ratings alone must
not be flagged.

To use Claude, set `TRIAGE_PROVIDER=anthropic` and provide `ANTHROPIC_API_KEY`
in the untracked local `.env` or process environment. `ANTHROPIC_MODEL` defaults
to `claude-haiku-5-5`; `ANTHROPIC_API_URL` defaults to
`https://api.anthropic.com/v1/messages`. Both real providers report missing or
failed credentials instead of silently switching to the mock.

For an opt-in real Gemini smoke request (uses one API call), open a PowerShell
terminal in `backend`. This loads the key into that terminal's process
environment without printing it, then runs the opt-in test:

```powershell
$settings = Get-Content ..\.env
$keyLine = $settings | Where-Object { $_ -match '^GEMINI_API_KEY=' } | Select-Object -First 1
$env:GEMINI_API_KEY = (($keyLine -split '=', 2)[1]).Trim().Trim('"')
$modelLine = $settings | Where-Object { $_ -match '^GEMINI_MODEL=' } | Select-Object -First 1
$env:GEMINI_MODEL = (($modelLine -split '=', 2)[1]).Trim().Trim('"')
$env:GEMINI_SMOKE_TEST = "1"
go test ./internal/triage -run TestGeminiProviderSmoke -v
Remove-Item Env:GEMINI_API_KEY, Env:GEMINI_MODEL, Env:GEMINI_SMOKE_TEST
Remove-Variable settings, keyLine, modelLine
```

The normal test suites use fake providers and make no Gemini requests. To
exercise the actual website, keep the backend running as above, start the
frontend in a second terminal, sign in, submit a review, then open Track status
and refresh using its reference code. Classification runs asynchronously; the
final status and saved flag reason appear there.

`TRIAGE_WORKERS`, `TRIAGE_QUEUE_SIZE`, `TRIAGE_MAX_RETRIES`,
`TRIAGE_RETRY_BACKOFF`, `TRIAGE_PROCESSING_TIMEOUT`,
`TRIAGE_RECOVERY_INTERVAL`, and `TRIAGE_LEASE_DURATION` configure the worker
pool and processing recovery. `TRIAGE_DUPLICATE_THRESHOLD` configures the
word-similarity threshold (default `0.82`; separate from brigading thresholds).
Pending `submitted` reviews are rediscovered from MongoDB after startup and
periodically, so a full in-memory queue does not lose persisted submissions.

The brigading integration is deliberately a development-only no-op until the
separate FR-8 implementation is merged. It is not a brigading detector.
`BRIGADING_MODE=development-noop` is accepted only with `APP_ENV=development`;
production startup remains blocked until the real detector is wired in.

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

The feedback endpoint responds with HTTP 201, an anonymous reference code, and
the initial `submitted` status. Classification and duplicate checking happen
in the background; use the Track status view to retrieve the latest state and
any saved flag reason.

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