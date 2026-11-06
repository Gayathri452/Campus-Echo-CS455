# Least-privilege MongoDB access for triage

The API and AI triage worker use separate MongoDB connections. The API uses
`MONGODB_URI`; triage uses `TRIAGE_MONGODB_URI`. The triage connection is
restricted in code to the feedback repository and must authenticate as a
MongoDB user that cannot read identity collections.

## Create the restricted role and user

Enable MongoDB authentication and connect as a MongoDB administrator. In
`mongosh`, create a custom role scoped only to the feedback collection. Repeat
with the actual database name if it is not `campusecho`:

```javascript
use campusecho

db.createRole({
  role: "campusechoTriage",
  privileges: [
    {
      resource: { db: "campusecho", collection: "feedback" },
      actions: ["find", "update"]
    }
  ],
  roles: []
})

db.createUser({
  user: "campusecho_triage",
  pwd: passwordPrompt(),
  roles: [{ role: "campusechoTriage", db: "campusecho" }]
})
```

The API account needs the permissions required by the rest of the application;
do not use the triage account as the API account. Keep both passwords in a
local ignored `.env` or deployment secret manager, never in source control.

Configure the API:

```dotenv
MONGODB_URI=mongodb://<api-user>:<encoded-password>@localhost:27017/campusecho?authSource=campusecho
TRIAGE_MONGODB_URI=mongodb://campusecho_triage:<encoded-password>@localhost:27017/campusecho?authSource=campusecho
```

URL-encode reserved characters in credentials. Outside `APP_ENV=development`,
startup requires a non-empty triage URI different from the API URI. In
development, an omitted URI falls back to `MONGODB_URI` with a warning; that
fallback is for convenience only and does not provide least-privilege
isolation.

## Verify the permission boundary

The opt-in integration test requires an admin URI and the restricted URI for
an isolated test database where the custom triage role is also configured.
The test inserts disposable feedback and identity records, checks that triage
can read and update feedback, asserts that reading `users` returns MongoDB
authorization error code 13, and removes only its own records.

Set these variables in PowerShell, replacing placeholders with local values:

```powershell
$env:TRIAGE_MONGODB_TEST_ADMIN_URI = "mongodb://<test-admin>:<encoded-password>@localhost:27017/admin?authSource=admin"
$env:TRIAGE_MONGODB_TEST_URI = "mongodb://<test-triage-user>:<encoded-password>@localhost:27017/campusecho_triage_test?authSource=campusecho_triage_test"
$env:TRIAGE_MONGODB_TEST_DATABASE = "campusecho_triage_test"
Set-Location C:\Users\obulr\Desktop\CS455-Project\backend
go test ./internal/database -run '^TestTriageMongoLeastPrivilegeIntegration$' -v
```

Create the same `campusechoTriage` role and a test triage user in
`campusecho_triage_test` before running the test. Do not point this test at a
production database. The test is skipped unless all three variables are set.
