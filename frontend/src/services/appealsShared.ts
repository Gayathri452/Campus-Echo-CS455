// Shared contract for the moderator pages. Types mirror
// backend/internal/appeals and docs/moderator-api.md. Change this file only by
// agreement between both moderator developers, in its own small PR.
//
// Person A's calls go in appealsQueue.ts, Person B's in appealsResolve.ts.

export type TicketStatus = "under_appeal" | "locked" | "restored" | "deleted";
export type SortOrder = "asc" | "desc";

/** List shape: queue rows, active-appeals rows, and the result of a claim. */
export interface TicketSummary {
  id: string;
  referenceCode: string;
  courseId: string;
  year: number;
  professor: string;
  status: TicketStatus;
  lockedAt?: string;
  createdAt: string;
}

/** Detail shape for the ticket page. */
export interface TicketDetail extends TicketSummary {
  reviewId: string;
  feedbackText: string;
  flagReason: string;
  appealComment: string;
}

/** Thrown for any non-2xx response; `status` is the HTTP status code. */
export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

/**
 * Sends an authenticated request to the Go API and returns the parsed JSON body
 * (or undefined for an empty 204 response). Callers validate the shape.
 */
export async function apiRequest(
  path: string,
  accessToken: string,
  init: { method?: "GET" | "POST"; body?: unknown } = {},
): Promise<unknown> {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${accessToken}`,
  };
  if (init.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  const response = await fetch(path, {
    method: init.method ?? "GET",
    headers,
    body: init.body === undefined ? undefined : JSON.stringify(init.body),
  });

  const text = await response.text();
  let result: unknown;
  if (text !== "") {
    try {
      result = JSON.parse(text);
    } catch {
      result = undefined;
    }
  }

  if (!response.ok) {
    const message =
      isRecord(result) && typeof result.error === "string"
        ? result.error
        : "The request could not be completed.";
    throw new ApiError(response.status, message);
  }
  return result;
}
