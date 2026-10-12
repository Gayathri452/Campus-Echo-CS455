# Moderator appeals: contract and ownership

This is the agreed contract for the moderator pages (FR-10 to FR-13, NFR-3).
Both moderator developers code against it. Change it only by agreement, in its
own small PR to `feature/moderator`.

## Who owns what

| Area | Person A (pages 1 and 3) | Person B (page 2) |
| --- | --- | --- |
| Jira | SCRUM-10, SCRUM-11 | SCRUM-12, SCRUM-13 |
| Handlers | `backend/internal/httpapi/appeals_queue.go` | `backend/internal/httpapi/appeals_resolve.go` |
| Mongo queries | `backend/internal/database/appeals_queue.go` | `backend/internal/database/appeals_resolve.go` |
| API calls | `frontend/src/services/appealsQueue.ts` | `frontend/src/services/appealsResolve.ts` |
| Pages | `QueuePage.tsx`, `ActiveAppealsPage.tsx` | `TicketDetailPage.tsx`, `DeleteModal.tsx` |
| Tests | `appeals_queue_test.go` | `appeals_resolve_test.go` |

All pages live in `frontend/src/pages/moderator/`. Add more files freely; do not
edit the other person's files.

**Shared files, already edited once in the scaffold commit. Do not edit them
for moderator work:** `httpapi/server.go`, `httpapi/moderator_routes.go`,
`database/mongo.go`, `cmd/api/main.go`, `appeals/appeals.go`, `appeals/expiry.go`,
`frontend/src/App.tsx`, `pages/moderator/ModeratorApp.tsx`,
`services/appealsShared.ts`. If one of them must change, tell the other person
and put it in its own PR.

## Ticket lifecycle

```text
under_appeal --claim--> locked --restore--> restored (review published)
     ^                    |   \--delete---> deleted  (review removed, comment stored)
     +---- release, or 30 minutes without resolving ----+
```

Rules every query must follow:

- **Claimable** means `status = under_appeal`, OR `status = locked` and
  `lockedAt <= appeals.LockExpiryCutoff(now)`. Use this same rule for the queue
  and for claiming, so an expired lock is claimable even before the sweep runs.
- **Claim is one atomic update** (`FindOneAndUpdate` with the claimable filter
  inside the query). Never read then write. Exactly one of 50 concurrent claims
  must succeed (NFR-3). The losers get 409.
- **Only the lock holder** can get, release or resolve a ticket, and only while
  the lock is unexpired. Put the `lockedBy` and expiry check inside the update
  filter.
- **No student identity** appears in tickets or in anything these handlers read
  or write (NFR-1, NFR-2).
- The 30-minute rule lives in `appeals.LockDuration`. Never hard-code it.

## Ticket document (`appeals` collection)

Created by the student-appeal feature (SCRUM-9, another developer), which must
store exactly these fields. Display fields are copied onto the ticket when the
appeal is made.

| Field | Type | Notes |
| --- | --- | --- |
| `_id` | ObjectId | ticket id, exposed as `id` |
| `reviewId` | string | id of the review in `feedback` |
| `referenceCode` | string | anonymous reference code |
| `courseId`, `year`, `professor` | string, int, string | header line on every page |
| `status` | string | `under_appeal`, `locked`, `restored`, `deleted` |
| `feedbackText` | string | the review |
| `flagReason` | string | why triage flagged it |
| `appealComment` | string | the student's explanation |
| `createdAt` | date | queue ordering |
| `lockedBy`, `lockedAt` | string, date | set while `locked`, removed on release |
| `resolutionComment`, `resolvedAt` | string, date | set on resolve; the student-status feature reads the comment |

On resolve, Person B also updates the review in `feedback`: `restore` sets its
`status` to `published`, `delete` sets it to `deleted`.

## Endpoints

All require `Authorization: Bearer <token>` of a **moderator**. No token gives
401 and a student or administrator token gives 403. Errors are
`{"error": "message"}`. Base path `/api/v1`.

| Method and path | Owner | Success | Errors |
| --- | --- | --- | --- |
| `GET /appeals?sort=asc\|desc` | A | 200 `{"appeals": [Summary]}` | 400 bad `sort` |
| `POST /appeals/{id}/claim` | A | 200 `Summary` | 409 already claimed; 404 unknown id |
| `GET /appeals/mine` | A | 200 `{"appeals": [Summary]}` | |
| `GET /appeals/{id}` | B | 200 `Ticket` | 403 not the lock holder (also when the lock expired); 404 |
| `POST /appeals/{id}/release` | B | 204 | 403 |
| `POST /appeals/{id}/resolve` | B | 204 | 400 bad action or missing comment; 403 |

- `sort` defaults to `asc` (oldest first, FR-10). An empty list is `[]`, not
  `null`.
- The 409 message is `This ticket has already been claimed`.
- `resolve` body: `{"action": "restore"}` or
  `{"action": "delete", "comment": "why it breaks the guidelines"}`.
  `comment` is required for `delete`.

`Summary` (queue and active rows, and the claim result):

```json
{
  "id": "6720f1c2a9b3c4d5e6f70811",
  "referenceCode": "CE-7K2P-91XD",
  "courseId": "CS455",
  "year": 2026,
  "professor": "Prof. Rao",
  "status": "under_appeal",
  "lockedAt": "2026-10-12T10:00:00Z",
  "createdAt": "2026-10-12T05:00:00Z"
}
```

`lockedAt` appears only while locked. `Ticket` (detail page) is a `Summary` plus:

```json
{
  "reviewId": "6720f1c2a9b3c4d5e6f70800",
  "feedbackText": "The projects were too long.",
  "flagReason": "Possibly abusive language",
  "appealComment": "This is harsh but fair criticism."
}
```

`lockedBy`, `resolutionComment` and `resolvedAt` are stored but never returned.

## Page flow (frontend)

1. **Page 1, queue.** Click a row: `claimTicket`, then `onTicketClaimed(id)`
   opens page 2. On 409 show the message and refresh the list.
2. **Page 2, detail.** Send back, publish or delete. After success call
   `onDone("...")`: the shell shows the message and returns to page 1. Delete
   opens a modal (Cancel, Submit) with a required comment.
3. **Page 3, active appeals.** Rows open page 2 with `onOpenTicket(id)` and do
   not claim again.

There is no router (the repo has none, and the token is kept in memory, so
URLs would not survive a reload). `ModeratorApp.tsx` holds the current page.

## Testing with seed data

Appeal creation is another feature, so seed some tickets. With MongoDB
running, from `backend`:

```powershell
go run ./cmd/seed-appeals
```

It removes its earlier data and inserts six appeals: four ordinary ones, one
whose lock is already 45 minutes old (it must appear in the queue), and one
locked 5 minutes ago by someone else (it must not). Create a moderator with the
administrator page, as described in the README.
