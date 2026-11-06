import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TicketSummary } from "../../services/appealsShared";
import type { AuthSession } from "../../services/auth";
import { fetchMyTickets } from "../../services/appealsQueue";
import ActiveAppealsPage from "./ActiveAppealsPage";

vi.mock("../../services/appealsQueue", () => ({
  claimTicket: vi.fn(),
  fetchQueue: vi.fn(),
  fetchMyTickets: vi.fn(),
}));

const mockedFetchMyTickets = vi.mocked(fetchMyTickets);
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
    status: "locked",
    createdAt: "2025-01-01",
    ...overrides,
  };
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("ActiveAppealsPage", () => {
  it("shows loading text until tickets load", async () => {
    let finish!: (tickets: TicketSummary[]) => void;
    mockedFetchMyTickets.mockImplementation(
      () => new Promise<TicketSummary[]>((resolve) => { finish = resolve; }),
    );
    render(<ActiveAppealsPage session={session} onOpenTicket={vi.fn()} />);
    expect(screen.getByRole("status")).toHaveTextContent("Loading active appeals");
    finish([ticket()]);
    expect(await screen.findByText("CE-123")).toBeInTheDocument();
  });

  it("renders rows and opens the selected ticket", async () => {
    mockedFetchMyTickets.mockResolvedValue([ticket()]);
    const onOpenTicket = vi.fn();
    render(<ActiveAppealsPage session={session} onOpenTicket={onOpenTicket} />);
    fireEvent.click(await screen.findByRole("button", { name: /CS101.*CE-123/ }));
    expect(onOpenTicket).toHaveBeenCalledWith("appeal-1");
  });

  it("shows an empty state", async () => {
    mockedFetchMyTickets.mockResolvedValue([]);
    render(<ActiveAppealsPage session={session} onOpenTicket={vi.fn()} />);
    expect(await screen.findByText("You have no active appeals.")).toBeInTheDocument();
  });

  it("shows a load error instead of the empty state", async () => {
    mockedFetchMyTickets.mockRejectedValue(new Error("active unavailable"));
    render(<ActiveAppealsPage session={session} onOpenTicket={vi.fn()} />);
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent("active unavailable"));
    expect(screen.queryByText("You have no active appeals.")).not.toBeInTheDocument();
  });
});
