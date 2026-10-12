// Person B owns this file: API calls for the ticket detail page
// (SCRUM-12, SCRUM-13). Endpoint shapes are in docs/moderator-api.md.
//
// Implement with apiRequest from ./appealsShared and validate the response with
// isRecord, as services/feedback.ts does. Let ApiError propagate: a 403 or 404
// from fetchTicket means the lock expired or the ticket is no longer this
// moderator's, which TicketDetailPage should report via onDone instead of
// crashing.

import { apiRequest, isRecord } from "./appealsShared";
import type { TicketDetail } from "./appealsShared";

// GET /api/v1/appeals/{id} -> TicketDetail
export async function fetchTicket(
  accessToken: string,
  ticketId: string,
): Promise<TicketDetail> {
  const result = await apiRequest(
    `/api/v1/appeals/${encodeURIComponent(ticketId)}`,
    accessToken,
  );

  if (
    !isRecord(result) ||
    typeof result.id !== "string" ||
    typeof result.referenceCode !== "string" ||
    typeof result.courseId !== "string" ||
    typeof result.year !== "number" ||
    typeof result.professor !== "string" ||
    typeof result.status !== "string" ||
    !["under_appeal", "locked", "restored", "deleted"].includes(String(result.status)) ||
    typeof result.createdAt !== "string" ||
    typeof result.reviewId !== "string" ||
    typeof result.feedbackText !== "string" ||
    typeof result.flagReason !== "string" ||
    typeof result.appealComment !== "string"
  ) {
    throw new Error("The server returned an unexpected ticket.");
  }

  return {
    id: result.id,
    referenceCode: result.referenceCode,
    courseId: result.courseId,
    year: result.year,
    professor: result.professor,
    status: result.status as TicketDetail["status"],
    createdAt: result.createdAt,
    reviewId: result.reviewId,
    feedbackText: result.feedbackText,
    flagReason: result.flagReason,
    appealComment: result.appealComment,
  };
}

// POST /api/v1/appeals/{id}/release -> 204
export async function releaseTicket(
  accessToken: string,
  ticketId: string,
): Promise<void> {
  await apiRequest(
    `/api/v1/appeals/${encodeURIComponent(ticketId)}/release`,
    accessToken,
    { method: "POST" },
  );
}

// POST /api/v1/appeals/{id}/resolve with { action: "restore" } -> 204
export async function restoreTicket(
  accessToken: string,
  ticketId: string,
): Promise<void> {
  await apiRequest(
    `/api/v1/appeals/${encodeURIComponent(ticketId)}/resolve`,
    accessToken,
    { method: "POST", body: { action: "restore" } },
  );
}

// POST /api/v1/appeals/{id}/resolve with { action: "delete", comment } -> 204
// The comment is required and is what the student later sees.
export async function deleteTicket(
  accessToken: string,
  ticketId: string,
  comment: string,
): Promise<void> {
  await apiRequest(
    `/api/v1/appeals/${encodeURIComponent(ticketId)}/resolve`,
    accessToken,
    { method: "POST", body: { action: "delete", comment } },
  );
}
