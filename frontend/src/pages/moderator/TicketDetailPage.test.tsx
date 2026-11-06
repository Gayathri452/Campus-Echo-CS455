import { cleanup, render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, type TicketDetail } from "../../services/appealsShared";
import type { AuthSession } from "../../services/auth";
import {
  deleteTicket,
  fetchTicket,
  releaseTicket,
  restoreTicket,
} from "../../services/appealsResolve";
import TicketDetailPage from "./TicketDetailPage";

vi.mock("../../services/appealsResolve", () => ({
  deleteTicket: vi.fn(),
  fetchTicket: vi.fn(),
  releaseTicket: vi.fn(),
  restoreTicket: vi.fn(),
}));

const mockedFetchTicket = vi.mocked(fetchTicket);
const mockedReleaseTicket = vi.mocked(releaseTicket);
const mockedRestoreTicket = vi.mocked(restoreTicket);
const mockedDeleteTicket = vi.mocked(deleteTicket);
const session: AuthSession = {
  accessToken: "access",
  user: { id: "mod-1", email: "mod@example.com", role: "moderator", createdAt: "now" },
};

function ticket(overrides: Partial<TicketDetail> = {}): TicketDetail {
  return {
    id: "appeal-1",
    referenceCode: "CE-123",
    courseId: "CS101",
    year: 2025,
    professor: "Ada Lovelace",
    status: "locked",
    createdAt: "2025-01-01",
    reviewId: "review-1",
    feedbackText: "Review text",
    flagReason: "Flag explanation",
    appealComment: "Please review again",
    ...overrides,
  };
}

async function renderLoadedTicket(data = ticket()) {
  mockedFetchTicket.mockResolvedValue(data);
  const onDone = vi.fn();
  render(<TicketDetailPage session={session} ticketId="appeal-1" onDone={onDone} />);
  await screen.findByRole("heading", { name: "Appeal details" });
  return onDone;
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("TicketDetailPage", () => {
  it("renders all ticket fields and fallback values", async () => {
    await renderLoadedTicket(ticket({ year: 0, professor: "" }));
    expect(screen.getByText("CE-123")).toBeInTheDocument();
    expect(screen.getByText(/CS101 · Not provided · Not provided/)).toBeInTheDocument();
    expect(screen.getByText("Review text")).toBeInTheDocument();
    expect(screen.getByText("Flag explanation")).toBeInTheDocument();
    expect(screen.getByText("Please review again")).toBeInTheDocument();
  });

  it.each([403, 404])("calls onDone when loading returns %i", async (status) => {
    const onDone = vi.fn();
    mockedFetchTicket.mockRejectedValue(new ApiError(status, "server details"));
    render(<TicketDetailPage session={session} ticketId="appeal-1" onDone={onDone} />);
    await waitFor(() => expect(onDone).toHaveBeenCalledWith("This appeal is no longer assigned to you"));
  });

  it("shows an unexpected load error", async () => {
    mockedFetchTicket.mockRejectedValue(new Error("load failed"));
    render(<TicketDetailPage session={session} ticketId="appeal-1" onDone={vi.fn()} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("load failed");
  });

  it.each([
    ["release", "Send back to queue", mockedReleaseTicket, "Appeal sent back to the queue"],
    ["restore", "Publish review", mockedRestoreTicket, "Review published"],
  ] as const)("runs %s and completes the ticket", async (_name, label, action, message) => {
    const user = userEvent.setup();
    const onDone = await renderLoadedTicket();
    await user.click(screen.getByRole("button", { name: label }));
    expect(action).toHaveBeenCalledWith("access", "appeal-1");
    expect(onDone).toHaveBeenCalledWith(message);
  });

  it("deletes with the modal comment and completes the ticket", async () => {
    const user = userEvent.setup();
    const onDone = await renderLoadedTicket();
    await user.click(screen.getByRole("button", { name: "Delete review" }));
    await user.type(screen.getByLabelText("Resolution comment"), "reason");
    await user.click(screen.getByRole("button", { name: "Submit" }));
    expect(mockedDeleteTicket).toHaveBeenCalledWith("access", "appeal-1", "reason");
    expect(onDone).toHaveBeenCalledWith("Review deleted");
  });

  it.each([403, 404])("finishing an action with %i reports the lost assignment", async (status) => {
    const user = userEvent.setup();
    const onDone = await renderLoadedTicket();
    mockedRestoreTicket.mockRejectedValueOnce(new ApiError(status, "gone"));
    await user.click(screen.getByRole("button", { name: "Publish review" }));
    await waitFor(() => expect(onDone).toHaveBeenCalledWith("This appeal is no longer assigned to you"));
  });

  it("disables actions while an operation is pending", async () => {
    const user = userEvent.setup();
    await renderLoadedTicket();
    let finish!: () => void;
    mockedRestoreTicket.mockImplementationOnce(() => new Promise<void>((resolve) => { finish = resolve; }));
    await user.click(screen.getByRole("button", { name: "Publish review" }));
    expect(screen.getByRole("button", { name: "Send back to queue" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Delete review" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Publish review" })).toBeDisabled();
    finish();
  });

  it("KNOWN BUG: ticket stays visible after failed action", async () => {
    const user = userEvent.setup();
    await renderLoadedTicket();
    mockedRestoreTicket.mockRejectedValueOnce(new ApiError(500, "server failed"));
    await user.click(screen.getByRole("button", { name: "Publish review" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("server failed");
    expect(screen.getByText("Review text")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send back to queue" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Publish review" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Delete review" })).toBeInTheDocument();
  });
});
