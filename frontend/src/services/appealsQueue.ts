// Person A owns this file: API calls for the queue and active-appeals pages
// (SCRUM-10, SCRUM-11). Endpoint shapes are in docs/moderator-api.md.
//
// Implement with apiRequest from ./appealsShared and validate the response with
// isRecord, as services/feedback.ts does. The claim call must let ApiError
// (status 409) propagate so QueuePage can show
// "This ticket has already been claimed".

import {
  apiRequest,
  isRecord,
  type SortOrder,
  type TicketSummary,
} from "./appealsShared";

function isTicketSummary(value: unknown): value is TicketSummary {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.referenceCode === "string" &&
    typeof value.courseId === "string" &&
    typeof value.year === "number" &&
    Number.isInteger(value.year) &&
    typeof value.professor === "string" &&
    (value.status === "under_appeal" ||
      value.status === "locked" ||
      value.status === "restored" ||
      value.status === "deleted") &&
    (value.lockedAt === undefined || typeof value.lockedAt === "string") &&
    typeof value.createdAt === "string"
  );
}

function parseTicketList(result: unknown): TicketSummary[] {
  if (
    !isRecord(result) ||
    !Array.isArray(result.appeals) ||
    !result.appeals.every(isTicketSummary)
  ) {
    throw new Error("The server returned an unexpected response.");
  }
  return result.appeals;
}

// GET /api/v1/appeals?sort=asc|desc -> { appeals: TicketSummary[] }
export async function fetchQueue(
  accessToken: string,
  sort: SortOrder,
): Promise<TicketSummary[]> {
  const result = await apiRequest(
    `/api/v1/appeals?sort=${encodeURIComponent(sort)}`,
    accessToken,
  );
  return parseTicketList(result);
}

// POST /api/v1/appeals/{id}/claim -> TicketSummary
export async function claimTicket(
  accessToken: string,
  ticketId: string,
): Promise<TicketSummary> {
  const result = await apiRequest(
    `/api/v1/appeals/${encodeURIComponent(ticketId)}/claim`,
    accessToken,
    { method: "POST" },
  );
  if (!isTicketSummary(result)) {
    throw new Error("The server returned an unexpected response.");
  }
  return result;
}

// GET /api/v1/appeals/mine -> { appeals: TicketSummary[] }
export async function fetchMyTickets(
  accessToken: string,
): Promise<TicketSummary[]> {
  const result = await apiRequest("/api/v1/appeals/mine", accessToken);
  return parseTicketList(result);
}
