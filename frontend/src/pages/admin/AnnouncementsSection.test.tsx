import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { mockFetch } from "../../test/fetchMock";
import AnnouncementsSection from "./AnnouncementsSection";

const activeAnnouncement = {
  id: "active-1",
  title: "Feedback window open",
  message: "Submit feedback before Friday.",
  pinned: true,
  courseId: "CS455",
  createdAt: "2026-11-01T10:00:00Z",
};
const expiredAnnouncement = {
  id: "expired-1",
  title: "Old announcement",
  message: "This announcement has expired.",
  pinned: false,
  expiresAt: "2020-10-01T10:00:00Z",
  createdAt: "2026-10-01T10:00:00Z",
};
const createdAnnouncement = {
  id: "created-1",
  title: "Office hours",
  message: "Extra office hours this week.",
  pinned: true,
  courseId: "CS455",
  expiresAt: "2026-11-20T23:59:59.000Z",
  createdAt: "2026-11-06T10:00:00Z",
};

describe("AnnouncementsSection", () => {
  it("loads announcements and courses, then posts a pinned course notice with an expiry", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/v1/admin/announcements" && init?.method === "POST") {
        return new Response(JSON.stringify(createdAnnouncement), { status: 201 });
      }
      if (path === "/api/v1/admin/announcements") {
        return new Response(JSON.stringify({ announcements: [activeAnnouncement, expiredAnnouncement] }));
      }
      if (path === "/api/v1/courses") {
        return new Response(JSON.stringify({ courses: [{ courseId: "CS455", title: "Software Engineering", createdAt: "2026-10-01T00:00:00Z" }] }));
      }
      return new Response(JSON.stringify({ error: `Unexpected request ${path}` }), { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    const user = userEvent.setup();
    render(<AnnouncementsSection accessToken="admin-token" />);
    expect(await screen.findByText("Feedback window open")).toBeInTheDocument();
    expect(screen.getByText("Showing to students (1)")).toBeInTheDocument();
    expect(screen.getByText("Expired (1)")).toBeInTheDocument();
    await user.click(screen.getByText("Expired (1)"));
    expect(await screen.findByText("Old announcement")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Office hours" } });
    fireEvent.change(screen.getByPlaceholderText("Share honest, specific feedback about your courses before Friday. Reviews are anonymous."), { target: { value: "Extra office hours this week." } });
    await user.selectOptions(screen.getByLabelText("Course (optional)"), "CS455");
    const expiresOn = new Date();
    expiresOn.setDate(expiresOn.getDate() + 1);
    const expiry = `${expiresOn.getFullYear()}-${String(expiresOn.getMonth() + 1).padStart(2, "0")}-${String(expiresOn.getDate()).padStart(2, "0")}`;
    fireEvent.change(screen.getByLabelText("Show until (optional)"), { target: { value: expiry } });
    await user.click(screen.getByLabelText("Pin to the top"));
    await user.click(screen.getByRole("button", { name: "Post announcement" }));

    expect(await screen.findByText(/Office hours.*now showing to students/)).toBeInTheDocument();
    expect(screen.getByText("Showing to students (2)")).toBeInTheDocument();
    const posted = fetchMock.mock.calls.find(([, init]) => init?.method === "POST");
    expect(posted).toBeDefined();
    expect(JSON.parse(String(posted?.[1]?.body))).toMatchObject({
      title: "Office hours",
      message: "Extra office hours this week.",
      courseId: "CS455",
      pinned: true,
    });
    expect(Date.parse(JSON.parse(String(posted?.[1]?.body)).expiresAt)).not.toBeNaN();
  });

  it("supports cancellation, server errors, and refresh after removal", async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetch({
      "/api/v1/admin/announcements": { body: { announcements: [activeAnnouncement] } },
      "/api/v1/courses": { body: { courses: [] } },
      "/api/v1/admin/announcements/active-1": { status: 500, body: { error: "Announcement is in use" } },
    });
    const confirm = vi.spyOn(window, "confirm");
    confirm.mockReturnValueOnce(false).mockReturnValueOnce(true);
    render(<AnnouncementsSection accessToken="admin-token" />);
    await screen.findByText("Feedback window open");
    await user.click(screen.getByRole("button", { name: "Remove" }));
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);

    await user.click(screen.getByRole("button", { name: "Remove" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Announcement is in use");
    expect(screen.getByText("Feedback window open")).toBeInTheDocument();
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(true));
  });

  it("shows loading failures and falls back when loading the course list fails", async () => {
    mockFetch({
      "/api/v1/admin/announcements": { status: 503, body: { error: "Announcement store unavailable" } },
      "/api/v1/courses": { status: 503, body: { error: "Course store unavailable" } },
    });
    render(<AnnouncementsSection accessToken="admin-token" />);
    expect(await screen.findByRole("alert")).toHaveTextContent("Announcement store unavailable");
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "Fallback announcement" } });
    fireEvent.change(screen.getByPlaceholderText("Share honest, specific feedback about your courses before Friday. Reviews are anonymous."), { target: { value: "The course selector is optional." } });
    expect(screen.getByLabelText("Course (optional)")).toBeInTheDocument();
  });

  it("reports an error when posting an invalid announcement", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/v1/courses") return new Response(JSON.stringify({ courses: [] }));
      if (path === "/api/v1/admin/announcements" && init?.method === "POST") {
        return new Response(JSON.stringify({ error: "title is required" }), { status: 400 });
      }
      if (path === "/api/v1/admin/announcements") return new Response(JSON.stringify({ announcements: [] }));
      return new Response("{}", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<AnnouncementsSection accessToken="admin-token" />);
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: "A title" } });
    fireEvent.change(screen.getByPlaceholderText("Share honest, specific feedback about your courses before Friday. Reviews are anonymous."), { target: { value: "A message" } });
    await user.click(screen.getByRole("button", { name: "Post announcement" }));
    expect(await screen.findByText("title is required")).toBeInTheDocument();
  });
});
