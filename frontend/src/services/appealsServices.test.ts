import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ApiError,
  apiRequest,
  type TicketDetail,
  type TicketSummary,
} from "./appealsShared";
import { claimTicket, fetchMyTickets, fetchQueue } from "./appealsQueue";
import {
  deleteTicket,
  fetchTicket,
  releaseTicket,
  restoreTicket,
} from "./appealsResolve";

const summary: TicketSummary = {
  id: "ticket/1",
  referenceCode: "CE-123",
  courseId: "CS101",
  year: 2025,
  professor: "Ada Lovelace",
  status: "locked",
  createdAt: "2025-01-01T00:00:00Z",
};

function response(body: string, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    text: async () => body,
  } as Response;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("apiRequest", () => {
  it("sends a bearer token and only sets content type when there is a body", async () => {
    const fetchMock = vi.fn().mockResolvedValue(response("{}"));
    vi.stubGlobal("fetch", fetchMock);
    await apiRequest("/test", "secret");
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      method: "GET",
      headers: { Authorization: "Bearer secret" },
      body: undefined,
    });
    expect(fetchMock.mock.calls[0][1].headers).not.toHaveProperty("Content-Type");

    await apiRequest("/test", "secret", { method: "POST", body: { ok: true } });
    expect(fetchMock.mock.calls[1][1]).toMatchObject({
      method: "POST",
      headers: {
        Authorization: "Bearer secret",
        "Content-Type": "application/json",
      },
      body: `{"ok":true}`,
    });
  });

  it.each([
    ["204", response("", 204)],
    ["empty body", response("")],
  ])("returns undefined for %s", async (_name, result) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(result));
    await expect(apiRequest("/test", "token")).resolves.toBeUndefined();
  });

  it("throws ApiError with the status and server error", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response('{"error":"denied"}', 403)));
    await expect(apiRequest("/test", "token")).rejects.toMatchObject({
      name: "ApiError",
      status: 403,
      message: "denied",
    } satisfies Partial<ApiError>);
  });

  it("uses a fallback for non-JSON error bodies", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response("not json", 500)));
    await expect(apiRequest("/test", "token")).rejects.toMatchObject({
      status: 500,
      message: "The request could not be completed.",
    });
  });
});

describe("appeals queue service", () => {
  const invalidRows: Array<[string, unknown]> = [
    ["not an object", null],
    ["appeals is not an array", { appeals: {} }],
    ["missing id", { appeals: [{ ...summary, id: undefined }] }],
    ["year is a string", { appeals: [{ ...summary, year: "2025" }] }],
    ["year is not an integer", { appeals: [{ ...summary, year: 2025.5 }] }],
    ["unknown status", { appeals: [{ ...summary, status: "pending" }] }],
    ["lockedAt is a number", { appeals: [{ ...summary, lockedAt: 123 }] }],
  ];

  it.each(["queue", "mine"])("%s list parses valid payloads", async (list) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response(JSON.stringify({ appeals: [summary] }))));
    const tickets =
      list === "queue"
        ? await fetchQueue("token", "asc")
        : await fetchMyTickets("token");
    expect(tickets).toEqual([summary]);
  });

  it.each(invalidRows)("fetchQueue rejects %s", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response(JSON.stringify(payload))));
    await expect(fetchQueue("token", "asc")).rejects.toThrow("unexpected response");
  });

  it.each(invalidRows)("fetchMyTickets rejects %s", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response(JSON.stringify(payload))));
    await expect(fetchMyTickets("token")).rejects.toThrow("unexpected response");
  });

  it("claims a valid ticket and rejects an invalid result", async () => {
    const fetchMock = vi.fn().mockResolvedValue(response(JSON.stringify(summary)));
    vi.stubGlobal("fetch", fetchMock);
    await expect(claimTicket("token", summary.id)).resolves.toEqual(summary);
    expect(fetchMock.mock.calls[0][0]).toContain("ticket%2F1/claim");
    fetchMock.mockResolvedValue(response("{}"));
    await expect(claimTicket("token", "id")).rejects.toThrow("unexpected response");
  });
});

describe("appeals resolve service", () => {
  const detail: TicketDetail = {
    ...summary,
    reviewId: "review-1",
    feedbackText: "review text",
    flagReason: "flag",
    appealComment: "please restore",
  };

  it("fetches and maps a ticket, URL-encoding its id", async () => {
    const fetchMock = vi.fn().mockResolvedValue(response(JSON.stringify({ ...detail, lockedBy: "secret" })));
    vi.stubGlobal("fetch", fetchMock);
    await expect(fetchTicket("token", "id/with space")).resolves.toEqual(detail);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/appeals/id%2Fwith%20space");
  });

  it.each([
    ["missing referenceCode", { ...detail, referenceCode: undefined }],
    ["wrong courseId", { ...detail, courseId: 1 }],
    ["wrong year", { ...detail, year: "2025" }],
    ["unknown status", { ...detail, status: "pending" }],
    ["missing reviewId", { ...detail, reviewId: undefined }],
    ["wrong feedbackText", { ...detail, feedbackText: null }],
    ["missing flagReason", { ...detail, flagReason: undefined }],
    ["wrong appealComment", { ...detail, appealComment: 1 }],
  ])("rejects ticket with %s", async (_name, payload) => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response(JSON.stringify(payload))));
    await expect(fetchTicket("token", "id")).rejects.toThrow("unexpected ticket");
  });

  it.each([
    ["release", releaseTicket, "/release", undefined],
    ["restore", restoreTicket, "/resolve", { action: "restore" }],
    ["delete", (token: string, id: string) => deleteTicket(token, id, "because"), "/resolve", { action: "delete", comment: "because" }],
  ] as const)("sends %s with the expected request", async (_name, operation, suffix, body) => {
    const fetchMock = vi.fn().mockResolvedValue(response("", 204));
    vi.stubGlobal("fetch", fetchMock);
    if (_name === "delete") {
      await deleteTicket("token", "id/1", "because");
    } else {
      await operation("token", "id/1");
    }
    expect(fetchMock).toHaveBeenCalledWith(
      `/api/v1/appeals/id%2F1${suffix}`,
      expect.objectContaining({
        method: "POST",
        ...(body === undefined ? { body: undefined } : { body: JSON.stringify(body) }),
      }),
    );
  });
});
