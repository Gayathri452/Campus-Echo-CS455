// Announcements API. Mirrors backend/internal/httpapi/announcements.go.

export interface Announcement {
  id: string;
  title: string;
  message: string;
  courseId?: string;
  pinned: boolean;
  expiresAt?: string;
  createdAt: string;
}

export interface NewAnnouncement {
  title: string;
  message: string;
  courseId?: string;
  pinned: boolean;
  /** ISO 8601 timestamp, or omitted for no expiry. */
  expiresAt?: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isAnnouncement(value: unknown): value is Announcement {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.title === "string" &&
    typeof value.message === "string" &&
    (value.courseId === undefined || typeof value.courseId === "string") &&
    typeof value.pinned === "boolean" &&
    (value.expiresAt === undefined || typeof value.expiresAt === "string") &&
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

function parseList(result: unknown): Announcement[] {
  if (!isRecord(result) || !Array.isArray(result.announcements) || !result.announcements.every(isAnnouncement)) {
    throw new Error("The server returned an unexpected announcement list.");
  }
  return result.announcements;
}

/** Active announcements for the signed-in user (pinned first, newest first). */
export async function listAnnouncements(accessToken: string): Promise<Announcement[]> {
  return parseList(await request("/api/v1/announcements", accessToken, { method: "GET" }, "Could not load announcements."));
}

/** Every announcement, including expired ones (administrator only). */
export async function listAllAnnouncements(accessToken: string): Promise<Announcement[]> {
  return parseList(await request("/api/v1/admin/announcements", accessToken, { method: "GET" }, "Could not load announcements."));
}

export async function createAnnouncement(accessToken: string, input: NewAnnouncement): Promise<Announcement> {
  const result = await request(
    "/api/v1/admin/announcements",
    accessToken,
    { method: "POST", body: input },
    "Could not post the announcement.",
  );
  if (!isAnnouncement(result)) throw new Error("The server returned an unexpected announcement.");
  return result;
}

export async function deleteAnnouncement(accessToken: string, id: string): Promise<void> {
  await request(
    `/api/v1/admin/announcements/${encodeURIComponent(id)}`,
    accessToken,
    { method: "DELETE" },
    "Could not remove the announcement.",
  );
}
