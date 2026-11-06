import { beforeEach, describe, expect, it, vi } from "vitest";
import { headersOf, mockFetch } from "../test/fetchMock";
import {
  login,
  logout,
  provisionModerator,
  removeModerator,
  requestRegistrationOTP,
  verifyOTP,
} from "./auth";

const user = {
  id: "student-1",
  email: "student@iitk.ac.in",
  role: "student",
  createdAt: "2026-11-01T00:00:00Z",
};
const moderator = { ...user, role: "moderator", email: "moderator@iitk.ac.in" };
const session = {
  accessToken: "test-token",
  tokenType: "Bearer",
  expiresAt: "2026-11-07T00:00:00Z",
  user,
};

describe("authentication service", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it.each([
    ["student", "/api/auth/student/login"],
    ["administrator", "/api/auth/admin/login"],
    ["moderator", "/api/auth/moderator/login"],
  ] as const)("logs in with the %s endpoint and parses a session", async (role, path) => {
    const fetchMock = mockFetch({ [path]: { body: session } });

    await expect(login("student@iitk.ac.in", "password", role)).resolves.toEqual(session);
    expect(fetchMock.mock.calls[0][0]).toBe(path);
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({
      email: "student@iitk.ac.in",
      password: "password",
    });
  });

  it("requests and verifies a registration OTP using a cookie session", async () => {
    const fetchMock = mockFetch({
      "/api/auth/request-otp": { status: 204 },
      "/api/auth/verify-otp": { body: user },
    });

    await expect(requestRegistrationOTP("student@iitk.ac.in", "password", "password")).resolves.toBeUndefined();
    await expect(verifyOTP("student@iitk.ac.in", "123456", "password")).resolves.toEqual({ user });
    expect(headersOf(fetchMock, 0)).not.toHaveProperty("Authorization");
    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual([
      "/api/auth/request-otp",
      "/api/auth/verify-otp",
    ]);
  });

  it("sends a bearer token when provisioning a moderator and rejects other roles", async () => {
    const fetchMock = mockFetch({ "/api/v1/admin/moderators": { body: moderator } });
    await expect(provisionModerator("admin-token", moderator.email, "secure-password")).resolves.toEqual(moderator);
    expect(headersOf(fetchMock)).toHaveProperty("Authorization");
    mockFetch({ "/api/v1/admin/moderators": { body: user } });
    await expect(provisionModerator("admin-token", moderator.email, "secure-password")).rejects.toThrow(
      "unexpected moderator response",
    );
  });

  it("surfaces API errors and uses safe fallbacks for malformed responses", async () => {
    mockFetch({ "/api/auth/student/login": { status: 401, body: { error: "Invalid credentials" } } });
    await expect(login("student@iitk.ac.in", "wrong", "student")).rejects.toThrow("Invalid credentials");

    mockFetch({ "/api/auth/student/login": { status: 502, raw: "<html>gateway error</html>" } });
    await expect(login("student@iitk.ac.in", "wrong", "student")).rejects.toThrow(
      "authentication service returned an invalid response (502)",
    );

    mockFetch({ "/api/auth/student/login": { status: 401, body: {} } });
    await expect(login("student@iitk.ac.in", "wrong", "student")).rejects.toThrow(
      "The request could not be completed.",
    );
  });

  it("rejects malformed success payloads for sessions and users", async () => {
    mockFetch({ "/api/auth/student/login": { body: { ...session, tokenType: "Basic" } } });
    await expect(login("student@iitk.ac.in", "password", "student")).rejects.toThrow(
      "unexpected authentication response",
    );

    mockFetch({ "/api/auth/verify-otp": { body: { ...user, role: "unknown" } } });
    await expect(verifyOTP("student@iitk.ac.in", "123456", "password")).rejects.toThrow(
      "unexpected authentication response",
    );
  });

  it("converts aborted requests into a timeout message and propagates other network errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new DOMException("aborted", "AbortError")));
    await expect(login("student@iitk.ac.in", "password", "student")).rejects.toThrow(
      "authentication request timed out",
    );

    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("offline")));
    await expect(login("student@iitk.ac.in", "password", "student")).rejects.toThrow("offline");
  });

  it("removes moderator access with an encoded email and reports fallback API errors", async () => {
    const fetchMock = mockFetch({ "/api/v1/admin/moderators/a%2Bb%40example.com": { status: 204 } });
    await expect(removeModerator("admin-token", "a+b@example.com")).resolves.toBeUndefined();
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/admin/moderators/a%2Bb%40example.com");

    mockFetch({ "/api/v1/admin/moderators/a%2Bb%40example.com": { status: 500, raw: "not json" } });
    await expect(removeModerator("admin-token", "a+b@example.com")).rejects.toThrow(
      "Moderator account could not be removed.",
    );
  });

  it("logs out and reports unsuccessful session revocation", async () => {
    const fetchMock = mockFetch({ "/api/auth/logout": { status: 204 } });
    await expect(logout("session-token")).resolves.toBeUndefined();
    expect(headersOf(fetchMock)).toHaveProperty("Authorization");

    mockFetch({ "/api/auth/logout": { status: 500, body: { error: "internal detail" } } });
    await expect(logout("session-token")).rejects.toThrow("Could not end the session on the server.");
  });
});
