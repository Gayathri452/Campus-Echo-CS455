import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { mockFetch } from "../test/fetchMock";
import AuthPage from "./AuthPage";

describe("AuthPage", () => {
  it("shows login validation, role-specific help, and a server login error", async () => {
    const user = userEvent.setup();
    const onAuthenticated = vi.fn();
    mockFetch({ "/api/auth/admin/login": { status: 401, body: { error: "Invalid credentials" } } });
    render(<AuthPage notice="Your previous session ended." onAuthenticated={onAuthenticated} />);

    expect(screen.getByRole("status")).toHaveTextContent("Your previous session ended.");
    await user.click(screen.getByRole("tab", { name: "Sign in" }));
    await user.click(screen.getByRole("tab", { name: "Administrator" }));
    expect(screen.getByRole("heading", { name: "Administrator sign in" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Register" })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("Email address"), "not-an-email");
    await user.type(screen.getByLabelText("Password"), "password");
    fireEvent.submit(screen.getByRole("button", { name: "Sign in" }).closest("form") as HTMLFormElement);
    expect(await screen.findByRole("alert")).toHaveTextContent("Enter a valid email address.");
    expect(onAuthenticated).not.toHaveBeenCalled();

    await user.clear(screen.getByLabelText("Email address"));
    await user.type(screen.getByLabelText("Email address"), "admin@example.com");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Invalid credentials");
    expect(onAuthenticated).not.toHaveBeenCalled();
  });

  it("validates IITK registration fields and starts OTP verification", async () => {
    const user = userEvent.setup();
    const onAuthenticated = vi.fn();
    const fetchMock = mockFetch({
      "/api/auth/request-otp": { status: 204 },
      "/api/auth/verify-otp": { body: { id: "student-1", email: "student@iitk.ac.in", role: "student", createdAt: "2026-11-01T00:00:00Z" } },
    });
    render(<AuthPage notice="" onAuthenticated={onAuthenticated} />);

    await user.click(screen.getByRole("tab", { name: "Register" }));
    await user.type(screen.getByLabelText("IITK email"), "student@example.com");
    await user.type(screen.getByPlaceholderText("6 characters minimum"), "short");
    await user.type(screen.getByPlaceholderText("Re-enter your password"), "different");
    await user.click(screen.getByRole("button", { name: "Send verification code" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("valid IITK email");

    await user.clear(screen.getByLabelText("IITK email"));
    await user.type(screen.getByLabelText("IITK email"), " Student@IITK.ac.in ");
    await user.clear(screen.getByPlaceholderText("6 characters minimum"));
    await user.type(screen.getByPlaceholderText("6 characters minimum"), "a-secure-password");
    await user.clear(screen.getByPlaceholderText("Re-enter your password"));
    await user.type(screen.getByPlaceholderText("Re-enter your password"), "a-secure-password");
    await user.click(screen.getByRole("button", { name: "Send verification code" }));

    expect(await screen.findByRole("heading", { name: "Verify your email" })).toBeInTheDocument();
    expect(screen.getByText(/student@iitk\.ac\.in/)).toBeInTheDocument();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/auth/request-otp");
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({
      email: "student@iitk.ac.in",
      password: "a-secure-password",
      confirmPassword: "a-secure-password",
    });

    const code = screen.getByPlaceholderText("000000");
    await user.type(code, "12a34567");
    expect(code).toHaveValue("123456");
    await user.click(screen.getByRole("button", { name: "Verify and create account" }));
    await waitFor(() => expect(onAuthenticated).toHaveBeenCalledWith({ user: expect.objectContaining({ email: "student@iitk.ac.in" }) }));
    expect(fetchMock.mock.calls[1][0]).toBe("/api/auth/verify-otp");
  });

  it("lets a user edit registration details and rejects malformed verification codes", async () => {
    const user = userEvent.setup();
    mockFetch({ "/api/auth/request-otp": { status: 204 } });
    render(<AuthPage notice="" onAuthenticated={vi.fn()} />);

    await user.click(screen.getByRole("tab", { name: "Register" }));
    await user.type(screen.getByLabelText("IITK email"), "student@iitk.ac.in");
    await user.type(screen.getByPlaceholderText("6 characters minimum"), "a-secure-password");
    await user.type(screen.getByPlaceholderText("Re-enter your password"), "a-secure-password");
    await user.click(screen.getByRole("button", { name: "Send verification code" }));
    await screen.findByPlaceholderText("000000");
    await user.click(screen.getByRole("button", { name: "Edit registration details" }));
    expect(screen.getByLabelText("IITK email")).toHaveValue("student@iitk.ac.in");

    await user.click(screen.getByRole("button", { name: "Send verification code" }));
    await screen.findByPlaceholderText("000000");
    fireEvent.change(screen.getByPlaceholderText("000000"), { target: { value: "123" } });
    await user.click(screen.getByRole("button", { name: "Verify and create account" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("six-digit verification code");
  });

  it("reports network failures and resets the login form when roles change", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("network unavailable")));
    render(<AuthPage notice="" onAuthenticated={vi.fn()} />);

    await user.click(screen.getByRole("tab", { name: "Administrator" }));
    await user.click(screen.getByRole("tab", { name: "Moderator" }));
    expect(screen.getByRole("heading", { name: "Moderator sign in" })).toBeInTheDocument();
    await user.type(screen.getByLabelText("Email address"), "mod@example.com");
    await user.type(screen.getByLabelText("Password"), "password");
    await user.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("network unavailable");
  });
});
