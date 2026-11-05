import { beforeEach, describe, expect, it } from "vitest";
import { headersOf, mockFetch } from "../test/fetchMock";
import { createCourse, getCachedCourses, invalidateCourseCache, listCourses } from "./courses";

const cs455 = { courseId: "CS455", title: "Software Engineering", createdAt: "2026-10-01T00:00:00Z" };

describe("courses service", () => {
  beforeEach(() => invalidateCourseCache());

  it("passes the search term and token", async () => {
    const fetchMock = mockFetch({ "/api/v1/courses": { body: { courses: [cs455] } } });
    await expect(listCourses("tok", "  soft ")).resolves.toEqual([cs455]);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/courses?search=soft");
    expect(headersOf(fetchMock)).toMatchObject({ Authorization: "Bearer tok" });
  });

  it("works with a cookie-only session (no Authorization header)", async () => {
    const fetchMock = mockFetch({ "/api/v1/courses": { body: { courses: [cs455] } } });
    await expect(getCachedCourses(undefined)).resolves.toEqual([cs455]);
    expect(headersOf(fetchMock)).not.toHaveProperty("Authorization");
  });

  it("rejects an unexpected course list", async () => {
    mockFetch({ "/api/v1/courses": { body: { courses: [{ courseId: 1 }] } } });
    await expect(listCourses("tok")).rejects.toThrow("unexpected course list");
  });

  it("shares one request between dropdowns for the same session", async () => {
    const fetchMock = mockFetch({ "/api/v1/courses": { body: { courses: [cs455] } } });
    await getCachedCourses("tok");
    await getCachedCourses("tok");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await getCachedCourses("other-token");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("does not cache a failed request", async () => {
    let fetchMock = mockFetch({ "/api/v1/courses": { status: 500, body: { error: "db down" } } });
    await expect(getCachedCourses("tok")).rejects.toThrow("db down");
    fetchMock = mockFetch({ "/api/v1/courses": { body: { courses: [cs455] } } });
    await expect(getCachedCourses("tok")).resolves.toEqual([cs455]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("refreshes the cache after an administrator adds a course", async () => {
    let fetchMock = mockFetch({ "/api/v1/courses": { body: { courses: [] } } });
    await getCachedCourses("tok");
    mockFetch({ "/api/v1/admin/courses": { status: 201, body: cs455 } });
    await createCourse("tok", { courseId: "CS455", title: "Software Engineering" });
    fetchMock = mockFetch({ "/api/v1/courses": { body: { courses: [cs455] } } });
    await expect(getCachedCourses("tok")).resolves.toEqual([cs455]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
