import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { invalidateCourseCache, type CatalogCourse } from "../../services/courses";
import { mockFetch } from "../../test/fetchMock";
import { CourseIdInput, CourseTitleAutofill, matchCourses } from "./CourseSuggestions";

const course = (courseId: string, title: string, instructor?: string): CatalogCourse => ({
  courseId,
  title,
  instructor,
  createdAt: "2026-10-01T00:00:00Z",
});

const catalog = [
  course("CS455", "Software Engineering", "Prof. Rao"),
  course("CS345", "Algorithms II"),
  course("MTH101", "Calculus for CS students"),
  course("EE200", "Signals", "Prof. Cs Kumar"),
];

describe("matchCourses", () => {
  it("returns the whole catalog for an empty query", () => {
    expect(matchCourses(catalog, "   ")).toEqual(catalog);
  });

  it("ranks ID prefix, then ID contains, then title, then instructor", () => {
    const ids = matchCourses(catalog, "cs").map((c) => c.courseId);
    expect(ids).toEqual(["CS345", "CS455", "MTH101", "EE200"]);
  });

  it("ignores spaces and case in course IDs", () => {
    expect(matchCourses(catalog, "cs 455").map((c) => c.courseId)).toEqual(["CS455"]);
  });

  it("returns nothing when nothing matches", () => {
    expect(matchCourses(catalog, "zzz")).toEqual([]);
  });
});

function Harness({ accessToken, onPick }: { accessToken?: string; onPick?: (c: CatalogCourse) => void }) {
  const [value, setValue] = useState("");
  return (
    <>
      <CourseIdInput accessToken={accessToken} onChange={setValue} onPick={onPick} placeholder="Course ID" value={value} />
      <output data-testid="value">{value}</output>
    </>
  );
}

describe("CourseIdInput", () => {
  beforeEach(() => invalidateCourseCache());

  it("shows matching courses while typing and fills the one clicked", async () => {
    mockFetch({ "/api/v1/courses": { body: { courses: catalog } } });
    const onPick = vi.fn();
    const user = userEvent.setup();
    render(<Harness accessToken="tok" onPick={onPick} />);

    await user.type(screen.getByRole("combobox"), "455");
    const option = await screen.findByRole("option", { name: /Software Engineering/ });
    await user.click(option);

    expect(screen.getByTestId("value")).toHaveTextContent("CS455");
    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ courseId: "CS455" }));
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("supports arrow keys, Enter and Escape", async () => {
    mockFetch({ "/api/v1/courses": { body: { courses: catalog } } });
    const user = userEvent.setup();
    render(<Harness accessToken="tok" />);
    const input = screen.getByRole("combobox");

    await user.type(input, "cs");
    await screen.findByRole("listbox");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(screen.getByTestId("value")).toHaveTextContent("CS455");

    await user.clear(input);
    await user.type(input, "c");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("keeps free text for a course that is not on the list", async () => {
    mockFetch({ "/api/v1/courses": { body: { courses: catalog } } });
    const user = userEvent.setup();
    render(<Harness accessToken="tok" />);
    await waitFor(() => expect(fetch).toHaveBeenCalled());

    await user.type(screen.getByRole("combobox"), "PHY999");
    expect(await screen.findByText(/No course on the list matches/)).toBeInTheDocument();
    expect(screen.getByTestId("value")).toHaveTextContent("PHY999");
  });

  it("still works as a plain text box when the course list cannot load", async () => {
    mockFetch({ "/api/v1/courses": { status: 500, body: { error: "db down" } } });
    const user = userEvent.setup();
    render(<Harness accessToken="tok" />);
    await user.type(screen.getByRole("combobox"), "CS455");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(screen.getByTestId("value")).toHaveTextContent("CS455");
  });

  // Same root cause as the announcements bug: no suggestions after a
  // cookie-restored session because the hook exited without a token.
  it("loads suggestions after a cookie-restored session (no access token)", async () => {
    mockFetch({ "/api/v1/courses": { body: { courses: catalog } } });
    const user = userEvent.setup();
    render(<Harness />);
    await user.type(screen.getByRole("combobox"), "455");
    expect(await screen.findByRole("option", { name: /Software Engineering/ })).toBeInTheDocument();
  });
});

describe("CourseTitleAutofill", () => {
  beforeEach(() => invalidateCourseCache());

  it("fills an empty title when the course ID matches the list", async () => {
    mockFetch({ "/api/v1/courses": { body: { courses: catalog } } });
    const onFill = vi.fn();
    render(<CourseTitleAutofill accessToken="tok" courseId="cs455" courseTitle="" onFill={onFill} />);
    await waitFor(() => expect(onFill).toHaveBeenCalledWith("Software Engineering"));
  });

  it("never overwrites a title the student typed", async () => {
    const fetchMock = mockFetch({ "/api/v1/courses": { body: { courses: catalog } } });
    const onFill = vi.fn();
    render(<CourseTitleAutofill accessToken="tok" courseId="CS455" courseTitle="My own title" onFill={onFill} />);
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(onFill).not.toHaveBeenCalled();
  });
});
