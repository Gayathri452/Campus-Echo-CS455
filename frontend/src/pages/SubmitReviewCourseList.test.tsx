import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import type { AuthSession } from "../services/auth";
import { invalidateCourseCache } from "../services/courses";
import { mockFetch } from "../test/fetchMock";
import StudentDashboardPage from "./StudentDashboardPage";

// Bug 4: reviews are accepted only for courses on the administrator's list,
// and the course title comes from the list.

const session: AuthSession = {
  accessToken: "tok",
  tokenType: "Bearer",
  expiresAt: "2026-11-07T00:00:00Z",
  user: { id: "s1", email: "student@iitk.ac.in", role: "student", createdAt: "2026-11-01T00:00:00Z" },
};

const savedReview = {
  referenceCode: "CE-NEW123",
  courseId: "CS455",
  courseTitle: "Software Engineering",
  professor: "Prof. Rao",
  year: 2026,
  category: "teaching",
  rating: 4,
  content: "A useful course",
  status: "submitted",
  createdAt: "2026-11-06T00:00:00Z",
};

function routes() {
  return mockFetch({
    "/api/v1/announcements": { body: { announcements: [] } },
    "/api/v1/courses": {
      body: { courses: [{ courseId: "CS455", title: "Software Engineering", createdAt: "2026-10-01T00:00:00Z" }] },
    },
    "/api/v1/feedback": { status: 201, body: savedReview },
    "/api/v1/auth/me": { body: { id: "s1" } },
  });
}

function postedReviews(fetchMock: ReturnType<typeof routes>) {
  return fetchMock.mock.calls
    .filter(([url, init]) => url === "/api/v1/feedback" && (init as RequestInit | undefined)?.method === "POST")
    .map(([, init]) => JSON.parse(String((init as RequestInit).body)) as Record<string, unknown>);
}

async function openSubmitForm() {
  const user = userEvent.setup();
  render(<StudentDashboardPage onLogout={() => undefined} session={session} />);
  await user.click(screen.getByRole("button", { name: "Submit review" }));
  await user.type(screen.getByLabelText("Professor name"), "Prof. Rao");
  await user.click(screen.getByRole("button", { name: "4 / 5" }));
  await user.type(screen.getByLabelText("Your feedback"), "A useful course");
  return user;
}

describe("Submit review: course must be on the list", () => {
  beforeEach(() => invalidateCourseCache());

  it("does not send a review for a course that is not on the list", async () => {
    const fetchMock = routes();
    const user = await openSubmitForm();
    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).startsWith("/api/v1/courses"))).toBe(true));

    await user.type(screen.getByPlaceholderText("CS455"), "CS 455");
    await user.click(screen.getByRole("button", { name: "Submit anonymous review" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Choose a course from the list");
    expect(postedReviews(fetchMock)).toHaveLength(0);
  });

  it("sends the listed course ID and title, whatever case was typed", async () => {
    const fetchMock = routes();
    const user = await openSubmitForm();

    await user.type(screen.getByPlaceholderText("CS455"), "cs455");
    await waitFor(() => expect(screen.getByLabelText("Course title")).toHaveValue("Software Engineering"));
    expect(screen.getByLabelText("Course title")).toHaveAttribute("readonly");
    await user.click(screen.getByRole("button", { name: "Submit anonymous review" }));

    expect(await screen.findByText("Keep this reference code")).toBeInTheDocument();
    expect(postedReviews(fetchMock)).toEqual([
      expect.objectContaining({ courseId: "CS455", courseTitle: "Software Engineering" }),
    ]);
  });
});
