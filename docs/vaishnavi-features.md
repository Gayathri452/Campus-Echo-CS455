# Course list, announcements and the My codes notebook

Author: Vaishnavi. These features are new; none of them is in the Deliverable 1
requirements or sprint backlog. They are additive: no existing route, page or
behaviour changes.

| Feature | Who uses it | Main files |
| --- | --- | --- |
| Course list management | Administrator | `backend/internal/courses/`, `backend/internal/httpapi/courses.go`, `backend/internal/database/courses.go`, `frontend/src/pages/admin/CourseCatalogSection.tsx` |
| Course dropdown and title autofill | Student | `frontend/src/pages/student/CourseSuggestions.tsx` |
| CSE course list seeder | Developers | `backend/cmd/seed-courses/` |
| My codes notebook | Student | `frontend/src/pages/student/ReferenceNotebookPage.tsx`, `SaveToNotebook.tsx`, `frontend/src/services/referenceNotebook.ts` |
| Announcements | Administrator posts, students read | `backend/internal/announcements/`, `backend/internal/httpapi/announcements.go`, `backend/internal/database/announcements.go`, `frontend/src/pages/admin/AnnouncementsSection.tsx`, `frontend/src/pages/student/AnnouncementsPanel.tsx` |

## Course list

- `GET /api/v1/courses?search=&limit=` — any signed-in user. Returns
  `{"courses": [{"courseId", "title", "instructor", "createdAt"}]}` sorted by
  course ID (at most 500).
- `POST /api/v1/admin/courses` — administrator only. Body
  `{"courseId": "CS455A", "title": "Introduction to Software Engineering", "instructor": "optional"}`.
  Course IDs are stored upper-case; 1–20 letters, digits, spaces or hyphens.
  Duplicate → 409; invalid → 400.
- `DELETE /api/v1/admin/courses/{courseId}` — administrator only. 204, or 404.
  Reviews that name the course are not touched.
- New `courses` collection with a unique `courseId` index.
- The list is advisory: students still type freely and review submission does
  not check it.
- Students see the list as a dropdown on the Course ID inputs (submit form and
  explore filter); picking a course fills in an empty title.
- Seed the 54 CSE courses: `cd backend` then `go run ./cmd/seed-courses`.

## My codes notebook

A reference code is shown only once, so it is saved automatically after a
review is submitted. The My codes tab shows each code with its course, a
personal note, and the latest status, flag reason, appeal state and moderator
note, plus search, a text backup and a shortcut to Track status.

**Privacy design:** codes are stored only in the student's browser
(`localStorage`, one notebook per signed-in student), never on the server. A
server-side record of "student X owns code Y" would let anyone with database
access link students to their anonymous reviews, which NFR-1 forbids.

## Announcements

- `GET /api/v1/announcements` — any signed-in user; active announcements only,
  pinned first, then newest (at most 50).
- `GET /api/v1/admin/announcements` — administrator only; includes expired.
- `POST /api/v1/admin/announcements` — administrator only. Body
  `{"title", "message", "courseId" (optional), "pinned", "expiresAt" (optional, RFC 3339)}`.
  Title ≤ 120 characters, message ≤ 1000, expiry in the future and at most a
  year away.
- `DELETE /api/v1/admin/announcements/{id}` — administrator only. 204, or 404.
- New `announcements` collection.
- Students see active announcements on the Overview page and can hide one
  (remembered in their browser only).

## Lines added to existing files

- `backend/internal/httpapi/server.go`: `server.registerCourseRoutes(mux)`,
  `server.registerAnnouncementRoutes(mux)`.
- `frontend/src/pages/AdministratorPage.tsx`: two imports,
  `<AnnouncementsSection />`, `<CourseCatalogSection />`.
- `frontend/src/pages/StudentDashboardPage.tsx`: imports, My codes tab and
  page, `<AnnouncementsPanel />`, `<CourseOptions />`, `<CourseTitleAutofill />`,
  `<SaveToNotebook />`; `list`/`autoComplete` attributes on the two Course ID
  inputs; `"codes"` added to the `StudentView` page names.

## Tests

```
cd backend
go test ./internal/courses/ ./internal/announcements/ ./cmd/seed-courses/
go test ./internal/httpapi/ -run "Course|Announcement"
```

`TestCourseCatalogIntegration` and `TestAnnouncementsIntegration` run against
MongoDB when `MONGODB_TEST_URI` is set.
