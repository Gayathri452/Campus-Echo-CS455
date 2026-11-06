import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { invalidateCourseCache } from "../../services/courses";
import { addToNotebook, loadNotebook, updateNotebookEntry } from "../../services/referenceNotebook";
import { headersOf, mockFetch } from "../../test/fetchMock";
import ReferenceNotebookPage from "./ReferenceNotebookPage";

const owner = "student-1";

function review(referenceCode: string, status: string, extra: Record<string, unknown> = {}) {
  return {
    referenceCode,
    courseId: "CS455",
    courseTitle: "Software Engineering",
    professor: "Prof. Rao",
    year: 2026,
    category: "teaching",
    rating: 4,
    content: "Good course",
    status,
    createdAt: "2026-11-01T10:00:00Z",
    ...extra,
  };
}

describe("ReferenceNotebookPage (My codes)", () => {
  beforeEach(() => invalidateCourseCache());

  it("shows an empty state when nothing is saved", () => {
    mockFetch({ "/api/v1/courses": { body: { courses: [] } } });
    render(<ReferenceNotebookPage ownerId={owner} accessToken="tok" onTrack={vi.fn()} />);
    expect(screen.getByText("No saved codes on this device")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Check all statuses" })).toBeDisabled();
  });

  it("automatically refreshes codes that have not been checked", async () => {
    addToNotebook(owner, { referenceCode: "AAA111" });
    const fetchMock = mockFetch({
      "/api/v1/courses": { body: { courses: [] } },
      "/api/v1/feedback/status/AAA111": { body: review("AAA111", "published") },
    });
    render(<ReferenceNotebookPage ownerId={owner} accessToken="tok" onTrack={vi.fn()} />);
    expect(await screen.findByText("Published")).toBeInTheDocument();
    const statusCall = fetchMock.mock.calls.findIndex(([url]) => String(url).includes("/feedback/status/"));
    expect(headersOf(fetchMock, statusCall)).toMatchObject({ Authorization: "Bearer tok" });
    expect(loadNotebook(owner)[0]).toMatchObject({ lastStatus: "published", courseId: "CS455" });
  });

  // Regression test for: "My codes automatic status refresh does not run after
  // cookie-based session restoration because it requires accessToken."
  it("automatically refreshes codes after a cookie-restored session (no access token)", async () => {
    addToNotebook(owner, { referenceCode: "AAA111" });
    const fetchMock = mockFetch({
      "/api/v1/courses": { body: { courses: [] } },
      "/api/v1/feedback/status/AAA111": { body: review("AAA111", "flagged", { flagReason: "Personal attack" }) },
    });
    render(<ReferenceNotebookPage ownerId={owner} onTrack={vi.fn()} />);
    expect(await screen.findByText("Flagged for review")).toBeInTheDocument();
    expect(screen.getByText("Personal attack")).toBeInTheDocument();
    const statusCall = fetchMock.mock.calls.findIndex(([url]) => String(url).includes("/feedback/status/"));
    expect(headersOf(fetchMock, statusCall)).not.toHaveProperty("Authorization");
  });

  it("does not re-check codes that were checked recently", async () => {
    addToNotebook(owner, { referenceCode: "AAA111" });
    updateNotebookEntry(owner, "AAA111", { lastStatus: "published", checkedAt: new Date().toISOString() });
    const fetchMock = mockFetch({ "/api/v1/courses": { body: { courses: [] } } });
    render(<ReferenceNotebookPage ownerId={owner} accessToken="tok" onTrack={vi.fn()} />);
    expect(await screen.findByText("Published")).toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(fetchMock.mock.calls.some(([url]) => String(url).includes("/feedback/status/"))).toBe(false);
  });

  it("shows an error for a code the server cannot find, and keeps the code", async () => {
    addToNotebook(owner, { referenceCode: "BAD999" });
    mockFetch({
      "/api/v1/courses": { body: { courses: [] } },
      "/api/v1/feedback/status/BAD999": { status: 404, body: { error: "review not found" } },
    });
    render(<ReferenceNotebookPage ownerId={owner} accessToken="tok" onTrack={vi.fn()} />);
    expect(await screen.findByText(/review not found/)).toBeInTheDocument();
    expect(loadNotebook(owner).map((e) => e.referenceCode)).toEqual(["BAD999"]);
  });

  it("filters saved codes by the search box (shown once there are more than 3)", async () => {
    addToNotebook(owner, { referenceCode: "AAA111", courseId: "CS455" });
    addToNotebook(owner, { referenceCode: "BBB222", courseId: "MTH101" });
    addToNotebook(owner, { referenceCode: "CCC333", courseId: "CS345" });
    addToNotebook(owner, { referenceCode: "DDD444", courseId: "EE200" });
    for (const code of ["AAA111", "BBB222", "CCC333", "DDD444"]) {
      updateNotebookEntry(owner, code, { lastStatus: "published", checkedAt: new Date().toISOString() });
    }
    mockFetch({ "/api/v1/courses": { body: { courses: [] } } });
    const user = userEvent.setup();
    render(<ReferenceNotebookPage ownerId={owner} accessToken="tok" onTrack={vi.fn()} />);

    await user.type(screen.getByPlaceholderText("Search codes, courses or notes"), "mth");
    await waitFor(() => expect(screen.queryByText("AAA111")).not.toBeInTheDocument());
    expect(screen.getByText("BBB222")).toBeInTheDocument();
  });
});
