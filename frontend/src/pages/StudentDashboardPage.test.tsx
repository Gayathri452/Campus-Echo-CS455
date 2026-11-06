import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { AuthSession } from "../services/auth";
import { invalidateCourseCache } from "../services/courses";
import { mockFetch } from "../test/fetchMock";
import StudentDashboardPage from "./StudentDashboardPage";

const session: AuthSession = {
  accessToken: "student-token",
  tokenType: "Bearer",
  expiresAt: "2026-11-07T00:00:00Z",
  user: {
    id: "student-1",
    email: "student@iitk.ac.in",
    role: "student",
    createdAt: "2026-11-01T00:00:00Z",
  },
};
const review = {
  referenceCode: "CE-ABC123",
  courseId: "CS455",
  courseTitle: "Software Engineering",
  professor: "Prof. Rao",
  year: 2026,
  category: "teaching",
  rating: 4,
  content: "The projects are helpful and well explained.",
  status: "published",
  createdAt: "2026-11-01T00:00:00Z",
};
const flaggedReview = { ...review, status: "flagged", flagReason: "Needs moderator review." };
const courses = {
  courses: [{ courseId: "CS455", title: "Software Engineering", createdAt: "2026-10-01T00:00:00Z" }],
};

function defaultRoutes(status: unknown = review) {
  return {
    "/api/v1/announcements": { body: { announcements: [] } },
    "/api/v1/courses": { body: courses },
    "/api/v1/feedback/explore": { body: [review] },
    "/api/v1/feedback/status/CE-ABC123": { body: status },
    "/api/v1/feedback/status/CE-ABC123/appeal": {
      status: 201,
      body: {
        referenceCode: review.referenceCode,
        reason: "Please reconsider this review.",
        status: "under_appeal",
        createdAt: "2026-11-02T00:00:00Z",
      },
    },
    "/api/v1/feedback": { status: 201, body: { ...review, status: "submitted" } },
  };
}

describe("StudentDashboardPage", () => {
  beforeEach(() => invalidateCourseCache());

  it("navigates through the overview and signs out", async () => {
    const user = userEvent.setup();
    const onLogout = vi.fn();
    mockFetch(defaultRoutes());
    render(<StudentDashboardPage onLogout={onLogout} session={session} />);
    expect(await screen.findByRole("heading", { name: "Your feedback matters" })).toBeInTheDocument();

    await user.click(within(screen.getByRole("navigation", { name: "Student navigation" })).getByRole("button", { name: "Explore reviews" }));
    expect(await screen.findByRole("heading", { name: "Explore reviews" })).toBeInTheDocument();
    expect(await screen.findByText(review.content)).toBeInTheDocument();
    await user.click(within(screen.getByRole("navigation", { name: "Student navigation" })).getByRole("button", { name: "My codes" }));
    expect(await screen.findByRole("heading", { name: "My reference codes" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Overview" }));
    expect(await screen.findByRole("heading", { name: "Your feedback matters" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Sign out" }));
    expect(onLogout).toHaveBeenCalledOnce();
  });

  it("searches published reviews and presents an empty state and server error", async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetch(defaultRoutes());
    render(<StudentDashboardPage onLogout={() => undefined} session={session} />);
    await user.click(within(screen.getByRole("navigation", { name: "Student navigation" })).getByRole("button", { name: "Explore reviews" }));
    expect(await screen.findByText(review.content)).toBeInTheDocument();

    await user.type(screen.getByPlaceholderText("Search course or feedback"), "grading");
    await user.selectOptions(screen.getByDisplayValue("All categories"), "teaching");
    await user.click(screen.getByRole("button", { name: "Search" }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).includes("search=grading"))).toBe(true));

    mockFetch({ ...defaultRoutes(), "/api/v1/feedback/explore": { body: [] } });
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(await screen.findByText("No published reviews yet")).toBeInTheDocument();

    mockFetch({ ...defaultRoutes(), "/api/v1/feedback/explore": { status: 503, body: { error: "Review search unavailable" } } });
    await user.click(screen.getByRole("button", { name: "Search" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Review search unavailable");
  });

  it("validates status lookup, normalizes codes, and displays review status details", async () => {
    const user = userEvent.setup();
    mockFetch(defaultRoutes(flaggedReview));
    render(<StudentDashboardPage onLogout={() => undefined} session={session} />);
    await user.click(screen.getByRole("button", { name: "Track status" }));
    await user.click(screen.getByRole("button", { name: "Check status" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Enter the reference code");

    await user.type(screen.getByPlaceholderText("CE-XXXXXXXXXXXX"), "ce-abc123");
    await user.click(screen.getByRole("button", { name: "Check status" }));
    expect(await screen.findByRole("heading", { name: review.courseTitle })).toBeInTheDocument();
    expect(screen.getByText("Flag reason:")).toBeInTheDocument();
    expect(screen.getByText("Flagged for review")).toBeInTheDocument();
  });

  it("submits an appeal, handles errors, and displays the moderator status", async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetch(defaultRoutes(flaggedReview));
    render(<StudentDashboardPage onLogout={() => undefined} session={session} />);
    await user.click(screen.getByRole("button", { name: "Track status" }));
    await user.type(screen.getByPlaceholderText("CE-XXXXXXXXXXXX"), review.referenceCode);
    await user.click(screen.getByRole("button", { name: "Check status" }));
    await screen.findByRole("heading", { name: review.courseTitle });
    const reason = screen.getByPlaceholderText("Your appeal reason");
    await user.type(reason, "Please reconsider this review.");
    await user.click(screen.getByRole("button", { name: "Submit appeal" }));
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => init?.method === "POST")).toBe(true));
    expect(await screen.findByText("Waiting for a moderator")).toBeInTheDocument();
    expect(screen.queryByPlaceholderText("Your appeal reason")).not.toBeInTheDocument();
  });

  it("shows a failed review status without leaking into appeal actions", async () => {
    const user = userEvent.setup();
    const failed = { ...review, status: "triage_failed", processingError: "Moderation is temporarily unavailable." };
    mockFetch(defaultRoutes(failed));
    render(<StudentDashboardPage onLogout={() => undefined} session={session} />);
    await user.click(screen.getByRole("button", { name: "Track status" }));
    await user.type(screen.getByPlaceholderText("CE-XXXXXXXXXXXX"), review.referenceCode);
    await user.click(screen.getByRole("button", { name: "Check status" }));

    expect(await screen.findByText("Moderation needs attention")).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Moderation is temporarily unavailable.");
    expect(screen.queryByRole("heading", { name: "Request an appeal" })).not.toBeInTheDocument();
  });

  it("surfaces status and appeal API errors", async () => {
    const user = userEvent.setup();
    mockFetch({
      ...defaultRoutes(flaggedReview),
      "/api/v1/feedback/status/CE-ABC123": { status: 404, body: { error: "Review not found" } },
    });
    render(<StudentDashboardPage onLogout={() => undefined} session={session} />);
    await user.click(screen.getByRole("button", { name: "Track status" }));
    await user.type(screen.getByPlaceholderText("CE-XXXXXXXXXXXX"), review.referenceCode);
    await user.click(screen.getByRole("button", { name: "Check status" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Review not found");

    mockFetch({
      ...defaultRoutes(flaggedReview),
      "/api/v1/feedback/status/CE-ABC123/appeal": { status: 409, body: { error: "An appeal already exists" } },
    });
    await user.click(screen.getByRole("button", { name: "Check status" }));
    await screen.findByRole("heading", { name: review.courseTitle });
    await user.type(screen.getByPlaceholderText("Your appeal reason"), "Please reconsider.");
    await user.click(screen.getByRole("button", { name: "Submit appeal" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("An appeal already exists");
  });

  it("rejects a missing rating and handles failed review submissions", async () => {
    const user = userEvent.setup();
    mockFetch({
      ...defaultRoutes(),
      "/api/v1/feedback": { status: 503, body: { error: "Review storage is unavailable" } },
    });
    render(<StudentDashboardPage onLogout={() => undefined} session={session} />);
    await user.click(screen.getByRole("button", { name: "Submit review" }));
    await user.type(screen.getByPlaceholderText("CS455"), "CS455");
    await user.type(screen.getByLabelText("Professor name"), "Prof. Rao");
    await user.type(screen.getByLabelText("Your feedback"), "A useful review.");
    await user.click(screen.getByRole("button", { name: "Submit anonymous review" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Choose a rating");

    await user.click(screen.getByRole("button", { name: "5 / 5" }));
    await user.click(screen.getByRole("button", { name: "Submit anonymous review" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Review storage is unavailable");
  });
});
