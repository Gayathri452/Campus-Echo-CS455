import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, type TicketSummary } from "../../services/appealsShared";
import type { AuthSession } from "../../services/auth";
import { claimTicket, fetchQueue } from "../../services/appealsQueue";
import QueuePage from "./QueuePage";

vi.mock("../../services/appealsQueue", () => ({
  claimTicket: vi.fn(),
  fetchQueue: vi.fn(),
  fetchMyTickets: vi.fn(),
}));

const mockedFetchQueue = vi.mocked(fetchQueue);
const mockedClaimTicket = vi.mocked(claimTicket);
const session: AuthSession = {
  accessToken: "access",
  user: { id: "mod-1", email: "mod@example.com", role: "moderator", createdAt: "now" },
};

function ticket(overrides: Partial<TicketSummary> = {}): TicketSummary {
  return {
    id: "appeal-1",
    referenceCode: "CE-123",
    courseId: "CS101",
    year: 2025,
    professor: "Ada Lovelace",
    status: "under_appeal",
    createdAt: "2025-01-01",
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("QueuePage", () => {
  it("loads and renders a ticket row", async () => {
    mockedFetchQueue.mockResolvedValue([ticket()]);
    render(<QueuePage session={session} onTicketClaimed={vi.fn()} />);
    expect(await screen.findByRole("button", { name: /CS101.*CE-123/ })).toBeInTheDocument();
    expect(mockedFetchQueue).toHaveBeenCalledWith("access", "asc");
  });

  it("shows the empty state", async () => {
    mockedFetchQueue.mockResolvedValue([]);
    render(<QueuePage session={session} onTicketClaimed={vi.fn()} />);
    expect(await screen.findByText("No appeals are waiting.")).toBeInTheDocument();
  });

  it("shows a list error without the empty state", async () => {
    mockedFetchQueue.mockRejectedValue(new Error("queue unavailable"));
    render(<QueuePage session={session} onTicketClaimed={vi.fn()} />);
    expect(await screen.findByRole("alert")).toHaveTextContent("queue unavailable");
    expect(screen.queryByText("No appeals are waiting.")).not.toBeInTheDocument();
  });

  it("changes sort, marks the active button and does not refetch the active sort", async () => {
    const user = userEvent.setup();
    mockedFetchQueue.mockResolvedValue([]);
    render(<QueuePage session={session} onTicketClaimed={vi.fn()} />);
    const oldest = screen.getByRole("button", { name: "Oldest to latest" });
    const latest = screen.getByRole("button", { name: "Latest to oldest" });
    await waitFor(() => expect(mockedFetchQueue).toHaveBeenCalledTimes(1));
    expect(oldest).toHaveAttribute("aria-pressed", "true");
    await user.click(oldest);
    expect(mockedFetchQueue).toHaveBeenCalledTimes(1);
    await user.click(latest);
    await waitFor(() => expect(mockedFetchQueue).toHaveBeenCalledWith("access", "desc"));
    expect(latest).toHaveAttribute("aria-pressed", "true");
    await user.click(oldest);
    await waitFor(() => expect(mockedFetchQueue).toHaveBeenCalledWith("access", "asc"));
    expect(oldest).toHaveAttribute("aria-pressed", "true");
  });

  it("refreshes the queue", async () => {
    const user = userEvent.setup();
    mockedFetchQueue.mockResolvedValue([]);
    render(<QueuePage session={session} onTicketClaimed={vi.fn()} />);
    await waitFor(() => expect(mockedFetchQueue).toHaveBeenCalledTimes(1));
    await user.click(screen.getByRole("button", { name: "Refresh" }));
    await waitFor(() => expect(mockedFetchQueue).toHaveBeenCalledTimes(2));
  });

  it("reports a successful claim", async () => {
    const user = userEvent.setup();
    const onTicketClaimed = vi.fn();
    mockedFetchQueue.mockResolvedValue([ticket()]);
    mockedClaimTicket.mockResolvedValue(ticket());
    render(<QueuePage session={session} onTicketClaimed={onTicketClaimed} />);
    await user.click(await screen.findByRole("button", { name: /CS101.*CE-123/ }));
    expect(mockedClaimTicket).toHaveBeenCalledWith("access", "appeal-1");
    expect(onTicketClaimed).toHaveBeenCalledWith("appeal-1");
  });

  it("shows a 409 claim error and reloads the queue", async () => {
    const user = userEvent.setup();
    mockedFetchQueue.mockResolvedValue([ticket()]);
    mockedClaimTicket.mockRejectedValue(new ApiError(409, "conflict"));
    render(<QueuePage session={session} onTicketClaimed={vi.fn()} />);
    await user.click(await screen.findByRole("button", { name: /CS101.*CE-123/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("This ticket has already been claimed");
    await waitFor(() => expect(mockedFetchQueue).toHaveBeenCalledTimes(2));
  });

  it("shows another claim error without reloading", async () => {
    const user = userEvent.setup();
    mockedFetchQueue.mockResolvedValue([ticket()]);
    mockedClaimTicket.mockRejectedValue(new Error("claim failed"));
    render(<QueuePage session={session} onTicketClaimed={vi.fn()} />);
    await user.click(await screen.findByRole("button", { name: /CS101.*CE-123/ }));
    expect(await screen.findByRole("alert")).toHaveTextContent("claim failed");
    expect(mockedFetchQueue).toHaveBeenCalledTimes(1);
  });

  it("ignores a second click while a claim is pending", async () => {
    let finishClaim!: (result: TicketSummary) => void;
    mockedFetchQueue.mockResolvedValue([ticket(), ticket({ id: "appeal-2", referenceCode: "CE-456" })]);
    mockedClaimTicket.mockImplementation(
      () => new Promise<TicketSummary>((resolve) => { finishClaim = resolve; }),
    );
    render(<QueuePage session={session} onTicketClaimed={vi.fn()} />);
    const row = await screen.findByRole("button", { name: /CS101.*CE-123/ });
    fireEvent.click(row);
    fireEvent.click(row);
    expect(mockedClaimTicket).toHaveBeenCalledTimes(1);
    finishClaim(ticket());
  });
});
