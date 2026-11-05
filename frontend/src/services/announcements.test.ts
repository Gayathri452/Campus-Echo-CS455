import { describe, expect, it } from "vitest";
import { headersOf, mockFetch } from "../test/fetchMock";
import { createAnnouncement, deleteAnnouncement, listAnnouncements } from "./announcements";

const sample = {
  id: "a1",
  title: "Feedback window open",
  message: "Submit reviews by Friday.",
  pinned: true,
  createdAt: "2026-11-01T10:00:00Z",
};

describe("announcements service", () => {
  it("lists announcements with a bearer token", async () => {
    const fetchMock = mockFetch({ "/api/v1/announcements": { body: { announcements: [sample] } } });
    await expect(listAnnouncements("tok")).resolves.toEqual([sample]);
    expect(headersOf(fetchMock)).toMatchObject({ Authorization: "Bearer tok" });
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ credentials: "same-origin" });
  });

  it("lists announcements with a cookie-only session (no Authorization header)", async () => {
    const fetchMock = mockFetch({ "/api/v1/announcements": { body: { announcements: [sample] } } });
    await expect(listAnnouncements(undefined)).resolves.toEqual([sample]);
    expect(headersOf(fetchMock)).not.toHaveProperty("Authorization");
  });

  it("rejects a malformed list", async () => {
    mockFetch({ "/api/v1/announcements": { body: { announcements: [{ id: 1 }] } } });
    await expect(listAnnouncements("tok")).rejects.toThrow("unexpected announcement list");
  });

  it("surfaces the server's error message", async () => {
    mockFetch({ "/api/v1/admin/announcements": { status: 400, body: { error: "title is required" } } });
    await expect(createAnnouncement("tok", { title: "", message: "m", pinned: false })).rejects.toThrow(
      "title is required",
    );
  });

  it("falls back to a friendly message when the error body is not JSON", async () => {
    mockFetch({ "/api/v1/admin/announcements/x": { status: 502, raw: "<html>Bad gateway</html>" } });
    await expect(deleteAnnouncement("tok", "x")).rejects.toThrow("Could not remove the announcement.");
  });

  it("encodes the id when deleting", async () => {
    const fetchMock = mockFetch({ "/api/v1/admin/announcements/a%2Fb": { status: 204 } });
    await deleteAnnouncement("tok", "a/b");
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/admin/announcements/a%2Fb");
  });
});
