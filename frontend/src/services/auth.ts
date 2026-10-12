export type UserRole = "student" | "moderator" | "administrator";

export interface AuthUser {
  id: string;
  email: string;
  role: UserRole;
  createdAt: string;
}

export interface AuthSession {
  accessToken?: string;
  tokenType?: "Bearer";
  expiresAt?: string;
  user: AuthUser;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isAuthUser(value: unknown): value is AuthUser {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.email === "string" &&
    (value.role === "student" ||
      value.role === "moderator" ||
      value.role === "administrator") &&
    typeof value.createdAt === "string"
  );
}

function parseSession(value: unknown): AuthSession {
  if (
    !isRecord(value) ||
    typeof value.accessToken !== "string" ||
    value.tokenType !== "Bearer" ||
    typeof value.expiresAt !== "string" ||
    !isAuthUser(value.user)
  ) {
    throw new Error("The server returned an unexpected authentication response.");
  }
  return {
    accessToken: value.accessToken,
    tokenType: "Bearer",
    expiresAt: value.expiresAt,
    user: value.user,
  };
}

function parseUserSession(value: unknown): AuthSession {
  if (!isAuthUser(value)) {
    throw new Error("The server returned an unexpected authentication response.");
  }
  return { user: value };
}

async function requestJSON(
  path: string,
  body: unknown,
  token?: string,
): Promise<unknown> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  const controller = new AbortController();
  const timeout = window.setTimeout(() => controller.abort(), 15000);
  let response: Response;
  try {
    response = await fetch(path, {
      method: "POST",
      headers,
      credentials: "same-origin",
      body: JSON.stringify(body),
      signal: controller.signal,
    });
  } catch (error: unknown) {
    if (error instanceof DOMException && error.name === "AbortError") {
      throw new Error("The authentication request timed out. Check the email service and try again.");
    }
    throw error;
  } finally {
    window.clearTimeout(timeout);
  }
  const responseText = await response.text();
  let result: unknown;
  try {
    result = responseText ? JSON.parse(responseText) : null;
  } catch {
    throw new Error(
      `The authentication service returned an invalid response (${response.status}). Restart the backend and try again.`,
    );
  }

  if (!response.ok) {
    const message =
      isRecord(result) &&
      "error" in result &&
      typeof result.error === "string"
        ? result.error
        : "The request could not be completed.";
    throw new Error(message);
  }
  return result;
}

export async function requestRegistrationOTP(
  email: string,
  password: string,
  confirmPassword: string,
): Promise<void> {
  await requestJSON("/api/auth/request-otp", {
    email,
    password,
    confirmPassword,
  });
}

export async function verifyOTP(email: string, code: string, password: string) {
  return parseUserSession(
    await requestJSON("/api/auth/verify-otp", { email, code, password }),
  );
}

export async function login(
  email: string,
  password: string,
  role: UserRole,
) {
  const paths: Record<UserRole, string> = {
    student: "/api/auth/student/login",
    administrator: "/api/auth/admin/login",
    moderator: "/api/auth/moderator/login",
  };
  return parseSession(
    await requestJSON(paths[role], { email, password }),
  );
}

export async function provisionModerator(
  token: string,
  email: string,
  password: string,
) {
  const user = await requestJSON(
    "/api/v1/admin/moderators",
    { email, password },
    token,
  );
  if (!isAuthUser(user) || user.role !== "moderator") {
    throw new Error("The server returned an unexpected moderator response.");
  }
  return user;
}

export async function removeModerator(token: string, email: string): Promise<void> {
  const response = await fetch(
    `/api/v1/admin/moderators/${encodeURIComponent(email)}`,
    {
      method: "DELETE",
      headers: token ? { Authorization: `Bearer ${token}` } : undefined,
      credentials: "same-origin",
    },
  );
  if (!response.ok) {
    const result: unknown = await response.json().catch(() => null);
    const message =
      isRecord(result) && typeof result.error === "string"
        ? result.error
        : "Moderator account could not be removed.";
    throw new Error(message);
  }
}

export async function logout(token: string): Promise<void> {
  const response = await fetch("/api/auth/logout", {
    method: "POST",
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    credentials: "same-origin",
  });
  if (!response.ok) {
    throw new Error("Could not end the session on the server.");
  }
}
