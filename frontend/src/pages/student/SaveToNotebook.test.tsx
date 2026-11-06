import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { loadNotebook } from "../../services/referenceNotebook";
import { mockFetch } from "../../test/fetchMock";
import SaveToNotebook from "./SaveToNotebook";

describe("SaveToNotebook", () => {
  it("saves a new reference code to My codes automatically", async () => {
    mockFetch({ "/api/v1/auth/me": { body: { id: "s-token" } } });
    render(<SaveToNotebook accessToken="tok-save-1" courseId="CS455" courseTitle="SE" referenceCode="NEW123" status="submitted" />);
    expect(await screen.findByText(/Saved to My codes/)).toBeInTheDocument();
    expect(loadNotebook("s-token")[0]).toMatchObject({ referenceCode: "NEW123", courseId: "CS455", lastStatus: "submitted" });
  });

  // The code is shown only once, so it must be saved with a cookie-only
  // session too (same root cause as the announcements bug).
  it("saves the code with a cookie-restored session (no access token)", async () => {
    mockFetch({ "/api/v1/auth/me": { body: { id: "s-cookie" } } });
    render(<SaveToNotebook courseId="CS455" courseTitle="SE" referenceCode="NEW456" />);
    expect(await screen.findByText(/Saved to My codes/)).toBeInTheDocument();
    expect(loadNotebook("s-cookie")[0]?.referenceCode).toBe("NEW456");
  });

  it("tells the student to copy the code if it could not be saved", async () => {
    mockFetch({ "/api/v1/auth/me": { status: 401, body: { error: "invalid or expired session" } } });
    render(<SaveToNotebook accessToken="tok-save-2" courseId="" courseTitle="" referenceCode="NEW789" />);
    expect(await screen.findByText(/could not save the code/)).toBeInTheDocument();
  });
});
