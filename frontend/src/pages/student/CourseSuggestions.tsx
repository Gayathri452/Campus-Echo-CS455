import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode } from "react";
import { getCachedCourses, type CatalogCourse } from "../../services/courses";

// Course suggestions from the administrator's course list.
//
// <CourseIdInput> is a Course ID box with a dropdown: typing "cs" lists every
// listed course whose ID or title contains "cs", and clicking one (or using
// the arrow keys and Enter) fills it in. It stays free text, so a course that
// is not on the list can still be typed exactly as before.

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

/** Courses matching what was typed: IDs starting with it first, then IDs containing it, then titles. */
export function matchCourses(catalog: CatalogCourse[], typed: string): CatalogCourse[] {
  const query = typed.trim().toLowerCase().replace(/\s+/g, " ");
  if (!query) return catalog;
  const compact = query.replace(/\s/g, "");
  const rank = (course: CatalogCourse) => {
    const id = course.courseId.toLowerCase();
    const idCompact = id.replace(/\s/g, "");
    if (idCompact.startsWith(compact)) return 0;
    if (idCompact.includes(compact)) return 1;
    if (course.title.toLowerCase().includes(query)) return 2;
    if ((course.instructor ?? "").toLowerCase().includes(query)) return 3;
    return -1;
  };
  return catalog
    .map((course) => ({ course, score: rank(course) }))
    .filter((item) => item.score >= 0)
    .sort((a, b) => a.score - b.score || a.course.courseId.localeCompare(b.course.courseId))
    .map((item) => item.course);
}

function highlight(text: string, typed: string): ReactNode {
  const query = typed.trim();
  const index = query ? text.toLowerCase().indexOf(query.toLowerCase()) : -1;
  if (index < 0) return text;
  return (
    <>
      {text.slice(0, index)}
      <span className="font-bold text-ocean">{text.slice(index, index + query.length)}</span>
      {text.slice(index + query.length)}
    </>
  );
}

interface CourseIdInputProps {
  accessToken?: string;
  value: string;
  onChange: (courseId: string) => void;
  /** Called when a course is picked from the dropdown (after onChange). */
  onPick?: (course: CatalogCourse) => void;
  placeholder?: string;
  className?: string;
}

export function CourseIdInput({
  accessToken,
  value,
  onChange,
  onPick,
  placeholder,
  className = "input",
}: CourseIdInputProps) {
  const catalog = useCatalog(accessToken);
  const [isOpen, setIsOpen] = useState(false);
  const [active, setActive] = useState(0);
  const listId = useId();
  const listRef = useRef<HTMLUListElement>(null);

  const matches = matchCourses(catalog, value);
  const exact = matches.length === 1 && matches[0].courseId.toLowerCase() === value.trim().toLowerCase();
  const showList = isOpen && catalog.length > 0 && !exact;

  useEffect(() => {
    setActive(0);
  }, [value]);

  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>(`[data-index="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active]);

  function pick(course: CatalogCourse) {
    onChange(course.courseId);
    onPick?.(course);
    setIsOpen(false);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setIsOpen(true);
      setActive((current) => Math.min(current + 1, Math.max(matches.length - 1, 0)));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((current) => Math.max(current - 1, 0));
    } else if (event.key === "Enter" && showList && matches[active]) {
      event.preventDefault();
      pick(matches[active]);
    } else if (event.key === "Escape") {
      setIsOpen(false);
    }
  }

  return (
    <div className="relative">
      <input
        aria-activedescendant={showList && matches[active] ? `${listId}-${active}` : undefined}
        aria-autocomplete="list"
        aria-controls={listId}
        aria-expanded={showList}
        autoComplete="off"
        className={className}
        onBlur={() => setIsOpen(false)}
        onChange={(event) => {
          onChange(event.target.value);
          setIsOpen(true);
        }}
        onClick={() => setIsOpen(true)}
        onFocus={() => setIsOpen(true)}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        role="combobox"
        value={value}
      />
      {showList && (
        <ul
          className="absolute left-0 right-0 top-full z-30 mt-1 max-h-72 overflow-auto rounded-lg border border-ink/15 bg-white py-1 text-left shadow-lg"
          id={listId}
          ref={listRef}
          role="listbox"
        >
          {matches.length === 0 ? (
            <li className="px-4 py-3 text-sm text-ink/55">
              No course on the list matches “{value.trim()}”. You can still type it.
            </li>
          ) : (
            matches.map((course, index) => (
              <li
                aria-selected={index === active}
                className={`cursor-pointer px-4 py-2.5 ${index === active ? "bg-mist" : "hover:bg-mist/60"}`}
                data-index={index}
                id={`${listId}-${index}`}
                key={course.courseId}
                // Pick on mouse down, before the input's blur closes the list.
                onMouseDown={(event) => {
                  event.preventDefault();
                  pick(course);
                }}
                onMouseEnter={() => setActive(index)}
                role="option"
              >
                <span className="block text-sm font-semibold text-ink">{highlight(course.courseId, value)}</span>
                <span className="block text-sm text-ink/65">
                  {highlight(course.title, value)}
                  {course.instructor ? <span className="text-ink/45"> · {course.instructor}</span> : null}
                </span>
              </li>
            ))
          )}
        </ul>
      )}
    </div>
  );
}

/**
 * Fills the course title when the typed course ID matches a listed course. It
 * never overwrites a title the student typed: it only fills an empty title or
 * one it filled itself for a previous course ID. Renders nothing.
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
