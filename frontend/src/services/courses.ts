// Course catalog API (administrator-managed list of courses).
// Mirrors backend/internal/httpapi/courses.go.

export interface CatalogCourse {
  courseId: string;
  title: string;
  instructor?: string;
  createdAt: string;
}

export interface NewCourse {
  courseId: string;
  title: string;
  instructor?: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isCourse(value: unknown): value is CatalogCourse {
  return (
    isRecord(value) &&
    typeof value.courseId === "string" &&
    typeof value.title === "string" &&
    (value.instructor === undefined || typeof value.instructor === "string") &&
    typeof value.createdAt === "string"
  );
}

async function request(
  path: string,
  accessToken: string,
  init: { method: "GET" | "POST" | "DELETE"; body?: unknown },
  fallback: string,
): Promise<unknown> {
  const headers: Record<string, string> = { Authorization: `Bearer ${accessToken}` };
  if (init.body !== undefined) headers["Content-Type"] = "application/json";
  const response = await fetch(path, {
    method: init.method,
    headers,
    body: init.body === undefined ? undefined : JSON.stringify(init.body),
    credentials: "same-origin",
  });
  const text = await response.text();
  let result: unknown;
  try {
    result = text ? JSON.parse(text) : undefined;
  } catch {
    result = undefined;
  }
  if (!response.ok) {
    throw new Error(isRecord(result) && typeof result.error === "string" ? result.error : fallback);
  }
  return result;
}

/** GET /api/v1/courses — the whole catalog (up to 500), sorted by course ID. */
export async function listCourses(accessToken: string, search = ""): Promise<CatalogCourse[]> {
  const params = new URLSearchParams();
  if (search.trim()) params.set("search", search.trim());
  const query = params.toString();
  const result = await request(
    `/api/v1/courses${query ? `?${query}` : ""}`,
    accessToken,
    { method: "GET" },
    "Could not load the course list.",
  );
  if (!isRecord(result) || !Array.isArray(result.courses) || !result.courses.every(isCourse)) {
    throw new Error("The server returned an unexpected course list.");
  }
  return result.courses;
}

/** POST /api/v1/admin/courses (administrator only). */
export async function createCourse(accessToken: string, course: NewCourse): Promise<CatalogCourse> {
  const result = await request(
    "/api/v1/admin/courses",
    accessToken,
    { method: "POST", body: course },
    "Could not add the course.",
  );
  if (!isCourse(result)) throw new Error("The server returned an unexpected course.");
  invalidateCourseCache();
  return result;
}

/** DELETE /api/v1/admin/courses/{courseId} (administrator only). */
export async function deleteCourse(accessToken: string, courseId: string): Promise<void> {
  await request(
    `/api/v1/admin/courses/${encodeURIComponent(courseId)}`,
    accessToken,
    { method: "DELETE" },
    "Could not remove the course.",
  );
  invalidateCourseCache();
}

// The student pages show the same catalog in several dropdowns, so one fetch
// per minute is shared between them.
const cacheLifetimeMs = 60_000;
let cache: { token: string; fetchedAt: number; promise: Promise<CatalogCourse[]> } | null = null;

export function getCachedCourses(accessToken: string): Promise<CatalogCourse[]> {
  const now = Date.now();
  if (cache && cache.token === accessToken && now - cache.fetchedAt < cacheLifetimeMs) {
    return cache.promise;
  }
  const promise = listCourses(accessToken).catch((error: unknown) => {
    cache = null;
    throw error;
  });
  cache = { token: accessToken, fetchedAt: now, promise };
  return promise;
}

export function invalidateCourseCache() {
  cache = null;
}
