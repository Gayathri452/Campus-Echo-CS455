import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { mockFetch } from "../../test/fetchMock";
import AnnouncementsPanel from "./AnnouncementsPanel";

const announcements = [
  { id: "a1", title: "Exam week", message: "Reviews close Friday.", pinned: true, createdAt: "2026-11-01T10:00:00Z" },
  { id: "a2", title: "New courses", message: "CS455 added.", courseId: "CS455", pinned: false, createdAt: "2026-11-02T10:00:00Z" },
];

describe("AnnouncementsPanel", () => {
  it("shows announcements for a token session", async () => {
    mockFetch({ "/api/v1/announcements": { body: { announcements } } });
    render(<AnnouncementsPanel accessToken="tok" ownerId="s1" />);
    expect(await screen.findByText("Exam week")).toBeInTheDocument();
    expect(screen.getByText("Pinned announcement")).toBeInTheDocument();
    expect(screen.getByText("Announcement for CS455")).toBeInTheDocument();
  });

  // Regression test for: "Announcements do not load after refreshing a
  // cookie-restored session because the component exits when accessToken is absent."
  it("loads announcements after a cookie-restored session (no access token)", async () => {
    const fetchMock = mockFetch({ "/api/v1/announcements": { body: { announcements } } });
    render(<AnnouncementsPanel ownerId="s1" />);
    expect(await screen.findByText("Exam week")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("renders nothing when there are no announcements or the request fails", async () => {
    const fetchMock = mockFetch({ "/api/v1/announcements": { status: 500, body: { error: "boom" } } });
    const { container } = render(<AnnouncementsPanel accessToken="tok" ownerId="s1" />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 0)); // let the failed request settle
    expect(container).toBeEmptyDOMElement();
  });

  it("hides an announcement, remembers it per student, and can show it again", async () => {
    mockFetch({ "/api/v1/announcements": { body: { announcements } } });
    const user = userEvent.setup();
    const { unmount } = render(<AnnouncementsPanel accessToken="tok" ownerId="s1" />);

    await user.click(await screen.findByRole("button", { name: "Hide announcement: Exam week" }));
    expect(screen.queryByText("Reviews close Friday.")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show 1 hidden announcement" })).toBeInTheDocument();

    // Remembered after a reload for the same student...
    unmount();
    const second = render(<AnnouncementsPanel accessToken="tok" ownerId="s1" />);
    expect(await screen.findByText("New courses")).toBeInTheDocument();
    expect(screen.queryByText("Reviews close Friday.")).not.toBeInTheDocument();

    // ...and can be shown again.
    await user.click(screen.getByRole("button", { name: "Show 1 hidden announcement" }));
    await user.click(screen.getByRole("button", { name: "Show again" }));
    expect(screen.getByText("Reviews close Friday.")).toBeInTheDocument();
    second.unmount();

    // A different student on the same browser still sees it.
    render(<AnnouncementsPanel accessToken="tok" ownerId="s2" />);
    expect(await screen.findByText("Reviews close Friday.")).toBeInTheDocument();
  });
});
