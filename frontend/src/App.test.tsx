import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { mockFetch } from "./test/fetchMock";
import App from "./App";

describe("App", () => {
  it.each([
    ["student", "/api/auth/student/login"],
    ["administrator", "/api/auth/admin/login"],
    ["moderator", "/api/auth/moderator/login"],
  ] as const)("routes an authenticated %s to the matching application", async (role, loginPath) => {
    const user = {
      id: `${role}-1`,
      email: `${role}@iitk.ac.in`,
      role,
      createdAt: "2026-11-01T00:00:00Z",
    };
    const fetchMock = mockFetch({
      [loginPath]: {
        body: {
          accessToken: "test-token",
          tokenType: "Bearer",
          expiresAt: "2026-11-07T00:00:00Z",
          user,
        },
      },
      "/api/auth/logout": { status: 204 },
      "/api/v1/announcements": { body: { announcements: [] } },
      "/api/v1/admin/announcements": { body: { announcements: [] } },
      "/api/v1/courses": { body: { courses: [] } },
    });
    render(<App />);

    if (role !== "student") {
      await screen.findByRole("tab", { name: role === "administrator" ? "Administrator" : "Moderator" });
      fireEvent.click(screen.getByRole("tab", { name: role === "administrator" ? "Administrator" : "Moderator" }));
    }
    fireEvent.change(screen.getByPlaceholderText("you@example.com"), {
      target: { value: user.email },
    });
    fireEvent.change(screen.getByPlaceholderText("6 characters minimum"), {
      target: { value: "secure-password" },
    });
    fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form") as HTMLFormElement);

    if (role === "student") {
      expect(await screen.findByRole("heading", { name: "Your feedback matters" })).toBeInTheDocument();
      await screen.findByRole("button", { name: "Sign out" });
      fireEvent.click(screen.getByRole("button", { name: "Sign out" }));
      await waitFor(() => expect(screen.queryByRole("button", { name: "Sign out" })).not.toBeInTheDocument());
      expect(fetchMock.mock.calls.some(([url]) => url === "/api/auth/logout")).toBe(true);
    } else if (role === "administrator") {
      expect(await screen.findByRole("heading", { name: "Announcements" })).toBeInTheDocument();
    } else {
      expect(await screen.findByRole("navigation", { name: "Moderator" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Appeal queue" })).toBeInTheDocument();
    }
    expect(fetchMock.mock.calls[0][0]).toBe(loginPath);
  });
});
