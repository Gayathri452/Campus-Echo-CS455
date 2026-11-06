import { fireEvent, render, screen, act } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TicketSummary } from "../../services/appealsShared";
import AppealRow from "./AppealRow";

const now = new Date("2026-11-06T00:00:00.000Z");

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
  vi.useRealTimers();
});

describe("AppealRow", () => {
  it("renders the ticket details and calls onSelect", () => {
    const onSelect = vi.fn();
    render(<AppealRow ticket={ticket()} onSelect={onSelect} />);

    fireEvent.click(screen.getByRole("button", { name: /CS101.*2025.*Ada Lovelace.*CE-123/ }));
    expect(onSelect).toHaveBeenCalledOnce();
    expect(screen.queryByRole("timer")).not.toBeInTheDocument();
  });

  it.each([
    ["enabled", false],
    ["disabled", true],
  ])("renders an %s button when disabled is %s", (_name, disabled) => {
    render(<AppealRow ticket={ticket()} onSelect={vi.fn()} disabled={disabled} />);
    expect(screen.getByRole("button")).toHaveProperty("disabled", disabled);
  });

  it("omits a countdown for invalid expiry dates", () => {
    render(
      <AppealRow
        ticket={ticket({ lockExpiresAt: "not-a-date" })}
        onSelect={vi.fn()}
        showLockExpiry
      />,
    );
    expect(screen.queryByRole("timer")).not.toBeInTheDocument();
  });

  it("shows expired and urgent countdown states", () => {
    vi.useFakeTimers();
    vi.setSystemTime(now);

    const { rerender } = render(
      <AppealRow
        ticket={ticket({ lockExpiresAt: now.toISOString() })}
        onSelect={vi.fn()}
        showLockExpiry
      />,
    );
    expect(screen.getByRole("timer")).toHaveAccessibleName("Appeal lock expired");
    expect(screen.getByText("Expired")).toBeInTheDocument();

    rerender(
      <AppealRow
        ticket={ticket({ lockExpiresAt: new Date(now.getTime() + 4 * 60_000).toISOString() })}
        onSelect={vi.fn()}
        showLockExpiry
      />,
    );
    expect(screen.getByRole("timer")).toHaveAccessibleName("Appeal lock expires in 4:00");
    expect(screen.getByRole("timer").className).toContain("bg-amber-50");
  });

  it("counts down and switches to the expired state", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(now);
    render(
      <AppealRow
        ticket={ticket({ lockExpiresAt: new Date(now.getTime() + 2_000).toISOString() })}
        onSelect={vi.fn()}
        showLockExpiry
      />,
    );

    expect(screen.getByRole("timer")).toHaveAccessibleName("Appeal lock expires in 0:02");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2_000);
    });
    expect(screen.getByRole("timer")).toHaveAccessibleName("Appeal lock expired");
  });
});
