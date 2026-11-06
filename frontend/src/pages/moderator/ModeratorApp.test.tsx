import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TicketDetail, TicketSummary } from "../../services/appealsShared";
import type { AuthSession } from "../../services/auth";
import { claimTicket, fetchMyTickets, fetchQueue } from "../../services/appealsQueue";
import { fetchTicket, releaseTicket } from "../../services/appealsResolve";
import ModeratorApp from "./ModeratorApp";

vi.mock("../../services/appealsQueue", () => ({
  claimTicket: vi.fn(),
  fetchQueue: vi.fn(),
  fetchMyTickets: vi.fn(),
}));
vi.mock("../../services/appealsResolve", () => ({
  deleteTicket: vi.fn(),
  fetchTicket: vi.fn(),
  releaseTicket: vi.fn(),
  restoreTicket: vi.fn(),
}));

const mockedFetchQueue = vi.mocked(fetchQueue);
const mockedFetchMyTickets = vi.mocked(fetchMyTickets);
const mockedClaimTicket = vi.mocked(claimTicket);
const mockedFetchTicket = vi.mocked(fetchTicket);
const mockedReleaseTicket = vi.mocked(releaseTicket);
const session: AuthSession = {
  accessToken: "access",
  user: { id: "mod-1", email: "mod@example.com", role: "moderator", createdAt: "now" },
};

function summary(): TicketSummary {
  return {
    id: "appeal-1",
    referenceCode: "CE-123",
    courseId: "CS101",
    year: 2025,
    professor: "Ada Lovelace",
    status: "under_appeal",
    createdAt: "2025-01-01",
  };
}

function detail(): TicketDetail {
  return {
    ...summary(),
    status: "locked",
    reviewId: "review-1",
    feedbackText: "Review text",
    flagReason: "Flag explanation",
    appealComment: "Please review again",
  };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ModeratorApp", () => {
  it("navigates, opens claimed tickets, shows completion notice, clears it and signs out", async () => {
    const user = userEvent.setup();
    const onLogout = vi.fn();
    mockedFetchQueue.mockResolvedValue([summary()]);
    mockedFetchMyTickets.mockResolvedValue([summary()]);
    mockedClaimTicket.mockResolvedValue(summary());
    mockedFetchTicket.mockResolvedValue(detail());
    mockedReleaseTicket.mockResolvedValue();

    render(<ModeratorApp session={session} onLogout={onLogout} />);
    expect(await screen.findByRole("heading", { name: "Appeal queue" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Active appeals" }));
    expect(await screen.findByRole("heading", { name: "Active appeals" })).toBeInTheDocument();
    expect(await screen.findByText("CE-123")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Appeal queue" }));
    await user.click(await screen.findByRole("button", { name: /CS101.*CE-123/ }));
    expect(await screen.findByRole("heading", { name: "Appeal details" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Send back to queue" }));
    expect(await screen.findByRole("status")).toHaveTextContent("Appeal sent back to the queue");
    expect(await screen.findByRole("heading", { name: "Appeal queue" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Active appeals" }));
    expect(screen.queryByText("Appeal sent back to the queue")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Sign out" }));
    expect(onLogout).toHaveBeenCalledOnce();
    await waitFor(() => expect(mockedFetchQueue).toHaveBeenCalled());
    expect(mockedFetchMyTickets).toHaveBeenCalledWith("access");
  });

  it("opens a ticket from Active appeals", async () => {
    mockedFetchQueue.mockResolvedValue([]);
    mockedFetchMyTickets.mockResolvedValue([summary()]);
    mockedFetchTicket.mockResolvedValue(detail());
    render(<ModeratorApp session={session} onLogout={vi.fn()} />);
    fireEvent.click(await screen.findByRole("button", { name: "Active appeals" }));
    fireEvent.click(await screen.findByRole("button", { name: /CS101.*CE-123/ }));
    expect(await screen.findByRole("heading", { name: "Appeal details" })).toBeInTheDocument();
    expect(mockedFetchTicket).toHaveBeenCalledWith("access", "appeal-1");
  });
});
