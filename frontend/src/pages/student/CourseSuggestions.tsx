import { useEffect, useRef, useState } from "react";
import { getCachedCourses, type CatalogCourse } from "../../services/courses";

// Course suggestions from the administrator's course list.
//
// <CourseOptions> renders one hidden <datalist>. Any input with
// list={COURSE_LIST_ID} then shows the courses as a dropdown while the student
// types. Inputs stay free text, so a course missing from the list can still be
// typed exactly as before.

export const COURSE_LIST_ID = "campus-course-catalog";

function useCatalog(accessToken?: string) {
  const [catalog, setCatalog] = useState<CatalogCourse[]>([]);
  useEffect(() => {
    if (!accessToken) return;
    let cancelled = false;
    getCachedCourses(accessToken)
      .then((courses) => {
        if (!cancelled) setCatalog(courses);
      })
      .catch(() => {
        // Suggestions are optional; the forms keep working without them.
      });
    return () => {
      cancelled = true;
    };
  }, [accessToken]);
  return catalog;
}

export function CourseOptions({ accessToken }: { accessToken?: string }) {
  const catalog = useCatalog(accessToken);
  return (
    <datalist id={COURSE_LIST_ID}>
      {catalog.map((course) => (
        <option
          key={course.courseId}
          label={course.instructor ? `${course.title} (${course.instructor})` : course.title}
          value={course.courseId}
        />
      ))}
    </datalist>
  );
}

/**
 * Fills the course title when the course ID matches a listed course. It never
 * overwrites a title the student typed: it only fills an empty title or one it
 * filled itself for a previous course ID. Renders nothing.
 */
export function CourseTitleAutofill({
  accessToken,
  courseId,
  courseTitle,
  onFill,
}: {
  accessToken?: string;
  courseId: string;
  courseTitle: string;
  onFill: (title: string) => void;
}) {
  const catalog = useCatalog(accessToken);
  const lastFilled = useRef("");

  useEffect(() => {
    const id = courseId.trim().toUpperCase().replace(/\s+/g, " ");
    const match = catalog.find((course) => course.courseId === id);
    const titleIsOurs = courseTitle.trim() === "" || courseTitle === lastFilled.current;
    if (match && titleIsOurs && courseTitle !== match.title) {
      lastFilled.current = match.title;
      onFill(match.title);
    }
    // onFill is a new function on every render of the parent form.
  }, [catalog, courseId, courseTitle]);

  return null;
}
