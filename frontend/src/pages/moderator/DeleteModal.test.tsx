import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import "@testing-library/jest-dom/vitest";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import DeleteModal from "./DeleteModal";

afterEach(cleanup);

describe("DeleteModal", () => {
  it.each(["", "   \t"])("keeps Submit disabled for %j", (comment) => {
    render(<DeleteModal isSubmitting={false} onCancel={vi.fn()} onSubmit={vi.fn()} />);
    fireEvent.change(screen.getByLabelText("Resolution comment"), {
      target: { value: comment },
    });
    expect(screen.getByRole("button", { name: "Submit" })).toBeDisabled();
  });

  it("enables Submit for a comment and submits the trimmed value", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<DeleteModal isSubmitting={false} onCancel={vi.fn()} onSubmit={onSubmit} />);
    await user.type(screen.getByLabelText("Resolution comment"), "  reason  ");
    const submit = screen.getByRole("button", { name: "Submit" });
    expect(submit).toBeEnabled();
    await user.click(submit);
    expect(onSubmit).toHaveBeenCalledWith("reason");
  });

  it.each(["cancel", "escape"])("%s calls onCancel", async (action) => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    render(<DeleteModal isSubmitting={false} onCancel={onCancel} onSubmit={vi.fn()} />);
    if (action === "cancel") {
      await user.click(screen.getByRole("button", { name: "Cancel" }));
    } else {
      fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    }
    expect(onCancel).toHaveBeenCalledOnce();
  });

  it("ignores Escape and disables buttons while submitting", () => {
    const onCancel = vi.fn();
    render(<DeleteModal isSubmitting onCancel={onCancel} onSubmit={vi.fn()} />);
    fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
    expect(onCancel).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Submitting..." })).toBeDisabled();
  });
});
