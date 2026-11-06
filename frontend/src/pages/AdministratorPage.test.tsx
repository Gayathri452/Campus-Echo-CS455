import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { AuthSession } from "../services/auth";
import AdministratorPage from "./AdministratorPage";

const session: AuthSession = {
  accessToken: "admin-token",
  tokenType: "Bearer",
  expiresAt: "2026-11-07T00:00:00Z",
  user: {
    id: "admin-1",
    email: "admin@example.com",
    role: "administrator",
    createdAt: "2026-11-01T00:00:00Z",
  },
};

describe("AdministratorPage", () => {
  it("navigates between administration sections and creates/removes moderators", async () => {
    const user = userEvent.setup();
    const onLogout = vi.fn();
    const moderator = {
      id: "moderator-1",
      email: "moderator@example.com",
      role: "moderator",
      createdAt: "2026-11-06T00:00:00Z",
    };
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/v1/admin/announcements") {
        return new Response(JSON.stringify({ announcements: [] }));
      }
      if (path === "/api/v1/courses") return new Response(JSON.stringify({ courses: [] }));
      if (path === "/api/v1/admin/moderators" && init?.method === "POST") {
        return new Response(JSON.stringify(moderator), { status: 201 });
      }
      if (path === "/api/v1/admin/moderators/moderator%40example.com" && init?.method === "DELETE") {
        return new Response(null, { status: 204 });
      }
      return new Response(JSON.stringify({ error: `Unexpected request ${path}` }), { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<AdministratorPage onLogout={onLogout} session={session} />);
    expect(await screen.findByRole("heading", { name: "Announcements" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Add moderator" }));
    fireEvent.change(screen.getByLabelText("Staff email"), { target: { value: moderator.email } });
    fireEvent.change(screen.getByLabelText(/Initial password/), { target: { value: "moderator-strong-password" } });
    await user.click(screen.getByRole("button", { name: "Create moderator" }));
    expect(await screen.findByText("Moderator account created for moderator@example.com.")).toBeInTheDocument();
    expect(JSON.parse(String(fetchMock.mock.calls.find(([, init]) => init?.method === "POST")?.[1]?.body))).toEqual({
      email: moderator.email,
      password: "moderator-strong-password",
    });

    fireEvent.change(screen.getByLabelText("Moderator email"), { target: { value: moderator.email } });
    await user.click(screen.getByRole("button", { name: "Remove moderator" }));
    expect(await screen.findByText("Moderator account removed for moderator@example.com.")).toBeInTheDocument();
    expect(screen.getByLabelText("Moderator email")).toHaveValue("");
    await waitFor(() => expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(true));

    await user.click(screen.getByRole("button", { name: "Add course" }));
    expect(await screen.findByRole("heading", { name: "Course list" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Announcements" }));
    expect(await screen.findByRole("heading", { name: "Announcements" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Sign out" }));
    expect(onLogout).toHaveBeenCalledOnce();
  });

  it("surfaces moderator provisioning and removal failures", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/v1/admin/announcements") return new Response(JSON.stringify({ announcements: [] }));
      if (path === "/api/v1/courses") return new Response(JSON.stringify({ courses: [] }));
      if (path === "/api/v1/admin/moderators" && init?.method === "POST") {
        return new Response(JSON.stringify({ error: "email already exists" }), { status: 409 });
      }
      if (path.includes("/api/v1/admin/moderators/")) {
        return new Response(JSON.stringify({ error: "moderator not found" }), { status: 404 });
      }
      return new Response("{}", { status: 404 });
    }));
    render(<AdministratorPage onLogout={() => undefined} session={session} />);
    await screen.findByRole("heading", { name: "Announcements" });
    await user.click(screen.getByRole("button", { name: "Add moderator" }));

    fireEvent.change(screen.getByLabelText("Staff email"), { target: { value: "moderator@example.com" } });
    fireEvent.change(screen.getByLabelText(/Initial password/), { target: { value: "moderator-strong-password" } });
    await user.click(screen.getByRole("button", { name: "Create moderator" }));
    expect(await screen.findByText("email already exists")).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Moderator email"), { target: { value: "missing@example.com" } });
    await user.click(screen.getByRole("button", { name: "Remove moderator" }));
    expect(await screen.findByText("moderator not found")).toBeInTheDocument();
  });
});
