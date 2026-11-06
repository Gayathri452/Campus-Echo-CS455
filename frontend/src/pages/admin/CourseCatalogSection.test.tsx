import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { invalidateCourseCache } from "../../services/courses";
import { mockFetch } from "../../test/fetchMock";
import CourseCatalogSection from "./CourseCatalogSection";

const cs340 = { courseId: "CS340", title: "Algorithms", instructor: "Prof. Iyer", createdAt: "2026-10-01T00:00:00Z" };
const cs455 = { courseId: "CS455", title: "Software Engineering", createdAt: "2026-10-02T00:00:00Z" };

describe("CourseCatalogSection", () => {
  it("adds a course, updates the list, and filters by instructor", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === "/api/v1/courses") {
        return new Response(JSON.stringify({ courses: [cs340] }));
      }
      if (String(input) === "/api/v1/admin/courses" && init?.method === "POST") {
        return new Response(JSON.stringify(cs455), { status: 201 });
      }
      return new Response("{}", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    invalidateCourseCache();
    render(<CourseCatalogSection accessToken="admin-token" />);
    expect(await screen.findByText("CS340")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Course ID"), "CS455");
    await user.type(screen.getByLabelText("Course title"), "Software Engineering");
    await user.click(screen.getByRole("button", { name: "Add course" }));
    expect(await screen.findByRole("status")).toHaveTextContent("CS455 — Software Engineering was added.");
    expect(screen.getByRole("heading", { name: "2 courses on the list" })).toBeInTheDocument();
    expect(fetchMock.mock.calls.find(([, init]) => init?.method === "POST")?.[1]?.headers).toMatchObject({
      "Content-Type": "application/json",
    });

    await user.type(screen.getByLabelText("Filter courses"), "iyer");
    expect(screen.getByText("CS340")).toBeInTheDocument();
    expect(screen.queryByText("CS455")).not.toBeInTheDocument();
  });

  it("handles catalog loading, filtering, and confirmed removal", async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetch({
      "/api/v1/courses": { body: { courses: [cs340, cs455] } },
      "/api/v1/admin/courses/CS340": { status: 204 },
    });
    vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true);
    render(<CourseCatalogSection accessToken="admin-token" />);
    expect(await screen.findByText("CS340")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Filter courses"), "missing");
    expect(screen.getByText('No course matches “missing”.')).toBeInTheDocument();
    await user.clear(screen.getByLabelText("Filter courses"));

    await user.click(screen.getAllByRole("button", { name: "Remove" })[0]);
    expect(fetchMock.mock.calls.some(([, init]) => init?.method === "DELETE")).toBe(false);
    await user.click(screen.getAllByRole("button", { name: "Remove" })[0]);
    expect(await screen.findByRole("status")).toHaveTextContent("CS340 was removed from the course list.");
    expect(screen.queryByText("CS340")).not.toBeInTheDocument();
  });

  it("adds spreadsheet rows, skips existing courses, and keeps failed rows for correction", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/v1/courses") return new Response(JSON.stringify({ courses: [cs340] }));
      if (path === "/api/v1/admin/courses" && init?.method === "POST") {
        const row = JSON.parse(String(init.body)) as { courseId: string; title: string; instructor?: string };
        if (row.courseId === "BAD101") {
          return new Response(JSON.stringify({ error: "title is invalid" }), { status: 400 });
        }
        return new Response(JSON.stringify({
          courseId: row.courseId.toUpperCase(),
          title: row.title,
          instructor: row.instructor,
          createdAt: "2026-11-01T00:00:00Z",
        }), { status: 201 });
      }
      return new Response("{}", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    invalidateCourseCache();
    render(<CourseCatalogSection accessToken="admin-token" />);
    await screen.findByText("CS340");
    const input = screen.getByLabelText(/One course per line/);
    await user.type(input, "CS340, duplicate title{enter}CS455, Software Engineering, Prof. Rao{enter}BAD101,");
    await user.click(screen.getByRole("button", { name: "Add all" }));

    expect(await screen.findByRole("status")).toHaveTextContent("Added 1 of 3. 1 already on the list. 1 need fixing");
    expect(screen.getByText("BAD101: title is invalid")).toBeInTheDocument();
    expect(input).toHaveValue("BAD101");
    expect(screen.getByRole("heading", { name: "2 courses on the list" })).toBeInTheDocument();
    await waitFor(() => expect(fetchMock.mock.calls.filter(([, init]) => init?.method === "POST")).toHaveLength(2));
  });

  it("shows loading errors and reports a failed course creation", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (String(input) === "/api/v1/courses") {
        return new Response(JSON.stringify({ error: "catalog offline" }), { status: 503 });
      }
      if (String(input) === "/api/v1/admin/courses" && init?.method === "POST") {
        return new Response(JSON.stringify({ error: "duplicate course ID" }), { status: 409 });
      }
      return new Response("{}", { status: 404 });
    });
    vi.stubGlobal("fetch", fetchMock);
    invalidateCourseCache();
    render(<CourseCatalogSection accessToken="admin-token" />);
    expect(await screen.findByText("catalog offline")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Course ID"), "CS455");
    await user.type(screen.getByLabelText("Course title"), "Software Engineering");
    await user.click(screen.getByRole("button", { name: "Add course" }));
    expect(await screen.findByText("duplicate course ID")).toBeInTheDocument();
  });
});
