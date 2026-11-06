import { describe, expect, it } from "vitest";
import { headersOf, mockFetch } from "../test/fetchMock";
import {
  addToNotebook,
  clearNotebook,
  getNotebookOwner,
  loadNotebook,
  normalizeReferenceCode,
  notebookAsText,
  removeFromNotebook,
  updateNotebookEntry,
} from "./referenceNotebook";

const owner = "student-1";

describe("reference notebook storage", () => {
  it("normalizes reference codes", () => {
    expect(normalizeReferenceCode("  ab 12-cd ")).toBe("AB12-CD");
  });

  it("starts empty and survives corrupt storage", () => {
    expect(loadNotebook(owner)).toEqual([]);
    window.localStorage.setItem("campusecho.referenceNotebook.v1:" + owner, "{not json");
    expect(loadNotebook(owner)).toEqual([]);
  });

  it("adds codes newest first and keeps them per student", () => {
    addToNotebook(owner, { referenceCode: "aaa111", courseId: "CS455", courseTitle: "Software Engineering" });
    addToNotebook(owner, { referenceCode: "bbb222" });
    const entries = loadNotebook(owner);
    expect(entries.map((e) => e.referenceCode)).toEqual(["BBB222", "AAA111"]);
    expect(entries[1]).toMatchObject({ courseId: "CS455", courseTitle: "Software Engineering", note: "" });
    expect(loadNotebook("someone-else")).toEqual([]);
  });

  it("does not duplicate a code saved twice and only fills missing course details", () => {
    addToNotebook(owner, { referenceCode: "AAA111", courseId: "CS455" });
    addToNotebook(owner, { referenceCode: "aaa111", courseId: "CS999", courseTitle: "Filled later" });
    const entries = loadNotebook(owner);
    expect(entries).toHaveLength(1);
    expect(entries[0]).toMatchObject({ courseId: "CS455", courseTitle: "Filled later" });
  });

  it("updates, truncates long notes, removes and clears", () => {
    addToNotebook(owner, { referenceCode: "AAA111" });
    addToNotebook(owner, { referenceCode: "BBB222" });
    const updated = updateNotebookEntry(owner, "AAA111", { note: "x".repeat(2500), lastStatus: "published" });
    const entry = updated.find((e) => e.referenceCode === "AAA111")!;
    expect(entry.note).toHaveLength(2000);
    expect(entry.lastStatus).toBe("published");

    expect(removeFromNotebook(owner, "AAA111").map((e) => e.referenceCode)).toEqual(["BBB222"]);
    clearNotebook(owner);
    expect(loadNotebook(owner)).toEqual([]);
  });

  it("exports a readable text backup", () => {
    addToNotebook(owner, { referenceCode: "AAA111", courseId: "CS455", courseTitle: "SE", lastStatus: "flagged" });
    updateNotebookEntry(owner, "AAA111", { note: "my CS455 review" });
    const text = notebookAsText(loadNotebook(owner));
    expect(text).toContain("AAA111");
    expect(text).toContain("Course: CS455 — SE");
    expect(text).toContain("Last status: flagged");
    expect(text).toContain("Note: my CS455 review");
  });
});

describe("getNotebookOwner", () => {
  it("sends the bearer token when one is available", async () => {
    const fetchMock = mockFetch({ "/api/v1/auth/me": { body: { id: "u1" } } });
    await expect(getNotebookOwner("token-a")).resolves.toBe("u1");
    expect(headersOf(fetchMock)).toMatchObject({ Authorization: "Bearer token-a" });
  });

  it("works with a cookie-only session (no token) and does not cache that answer", async () => {
    const fetchMock = mockFetch({ "/api/v1/auth/me": { body: { id: "u2" } } });
    await expect(getNotebookOwner(undefined)).resolves.toBe("u2");
    await expect(getNotebookOwner(undefined)).resolves.toBe("u2");
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(headersOf(fetchMock)).not.toHaveProperty("Authorization");
  });

  it("rejects when the server does not identify the student", async () => {
    mockFetch({ "/api/v1/auth/me": { status: 401, body: { error: "invalid or expired session" } } });
    await expect(getNotebookOwner("token-b")).rejects.toThrow("Could not identify the signed-in student.");
  });
});
