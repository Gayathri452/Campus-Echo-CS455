import { useEffect, useState, type FormEvent } from "react";
import {
  createCourse,
  deleteCourse,
  listCourses,
  type CatalogCourse,
} from "../../services/courses";

// Administrator view of the course catalog. Students see these courses as
// suggestions when they search for or review a course.

interface CourseCatalogSectionProps {
  accessToken: string;
}

const inputClass =
  "w-full rounded-lg border border-ink/15 px-4 py-3 outline-none focus:border-ocean focus:ring-2 focus:ring-ocean/15";

function CourseCatalogSection({ accessToken }: CourseCatalogSectionProps) {
  const [catalog, setCatalog] = useState<CatalogCourse[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [filter, setFilter] = useState("");
  const [removingId, setRemovingId] = useState("");
  const [listMessage, setListMessage] = useState("");

  async function refresh() {
    setIsLoading(true);
    setLoadError("");
    try {
      setCatalog(await listCourses(accessToken));
    } catch (error: unknown) {
      setLoadError(error instanceof Error ? error.message : "Could not load the course list.");
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // The access token only changes on a new sign-in, which remounts this page.
  }, [accessToken]);

  async function handleRemove(course: CatalogCourse) {
    if (!window.confirm(`Remove ${course.courseId} — ${course.title} from the course list? Existing reviews are not affected.`)) {
      return;
    }
    setRemovingId(course.courseId);
    setListMessage("");
    try {
      await deleteCourse(accessToken, course.courseId);
      setCatalog((current) => current.filter((item) => item.courseId !== course.courseId));
      setListMessage(`${course.courseId} was removed from the course list.`);
    } catch (error: unknown) {
      setListMessage(error instanceof Error ? error.message : "Could not remove the course.");
    } finally {
      setRemovingId("");
    }
  }

  function addToList(added: CatalogCourse[]) {
    setCatalog((current) =>
      [...current, ...added].sort((a, b) => a.courseId.localeCompare(b.courseId)),
    );
  }

  const needle = filter.trim().toLowerCase();
  const visible = catalog.filter(
    (course) =>
      !needle ||
      course.courseId.toLowerCase().includes(needle) ||
      course.title.toLowerCase().includes(needle) ||
      (course.instructor ?? "").toLowerCase().includes(needle),
  );

  return (
    <section className="mt-14 border-t border-ink/10 pt-10">
      <h2 className="text-2xl font-bold">Course list</h2>
      <p className="mt-3 max-w-2xl leading-7 text-ink/65">
        Courses added here appear as suggestions when students search for a course or write a
        review. Students can still type a course that is not on the list.
      </p>

      <div className="mt-8 grid gap-6 lg:grid-cols-2">
        <AddCourseForm accessToken={accessToken} onAdded={(course) => addToList([course])} />
        <BulkAddForm accessToken={accessToken} existing={catalog} onAdded={addToList} />
      </div>

      <div className="mt-8 rounded-2xl border border-ink/10 bg-white p-6 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h3 className="text-lg font-bold">
            {catalog.length} {catalog.length === 1 ? "course" : "courses"} on the list
          </h3>
          <div className="flex gap-2">
            <label className="sr-only" htmlFor="catalog-filter">Filter courses</label>
            <input
              className="w-56 rounded-lg border border-ink/15 px-3 py-2 text-sm outline-none focus:border-ocean focus:ring-2 focus:ring-ocean/15"
              id="catalog-filter"
              onChange={(event) => setFilter(event.target.value)}
              placeholder="Filter courses"
              value={filter}
            />
            <button
              className="rounded-lg border border-ink/15 px-4 py-2 text-sm font-semibold hover:bg-mist disabled:opacity-60"
              disabled={isLoading}
              onClick={() => void refresh()}
              type="button"
            >
              Refresh
            </button>
          </div>
        </div>

        {loadError && (
          <p className="mt-4 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{loadError}</p>
        )}
        {listMessage && (
          <p className="mt-4 rounded-lg bg-mist px-4 py-3 text-sm text-ink/75" role="status">{listMessage}</p>
        )}

        {isLoading ? (
          <p className="mt-4 text-sm text-ink/60" role="status">Loading courses…</p>
        ) : catalog.length === 0 && !loadError ? (
          <p className="mt-4 text-sm text-ink/60">
            No courses yet. Add one above, or paste a list with “Add several courses”.
          </p>
        ) : visible.length === 0 ? (
          <p className="mt-4 text-sm text-ink/60">No course matches “{filter}”.</p>
        ) : (
          <div className="mt-4 max-h-[28rem] overflow-auto">
            <table className="w-full text-left text-sm">
              <thead className="sticky top-0 bg-white">
                <tr className="border-b border-ink/10 text-ink/55">
                  <th className="py-2 pr-4 font-semibold">Course ID</th>
                  <th className="py-2 pr-4 font-semibold">Title</th>
                  <th className="py-2 pr-4 font-semibold">Instructor</th>
                  <th className="py-2"><span className="sr-only">Actions</span></th>
                </tr>
              </thead>
              <tbody>
                {visible.map((course) => (
                  <tr className="border-b border-ink/5 last:border-0" key={course.courseId}>
                    <td className="py-3 pr-4 font-semibold text-ocean">{course.courseId}</td>
                    <td className="py-3 pr-4">{course.title}</td>
                    <td className="py-3 pr-4 text-ink/65">{course.instructor || "—"}</td>
                    <td className="py-3 text-right">
                      <button
                        className="rounded-lg px-3 py-1.5 text-sm font-semibold text-rose-700 hover:bg-rose-50 disabled:opacity-60"
                        disabled={removingId === course.courseId}
                        onClick={() => void handleRemove(course)}
                        type="button"
                      >
                        {removingId === course.courseId ? "Removing…" : "Remove"}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </section>
  );
}

function AddCourseForm({
  accessToken,
  onAdded,
}: {
  accessToken: string;
  onAdded: (course: CatalogCourse) => void;
}) {
  const [form, setForm] = useState({ courseId: "", title: "", instructor: "" });
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [isSaving, setIsSaving] = useState(false);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setMessage("");
    setIsSaving(true);
    try {
      const created = await createCourse(accessToken, form);
      onAdded(created);
      setMessage(`${created.courseId} — ${created.title} was added.`);
      setForm({ courseId: "", title: "", instructor: "" });
    } catch (submitError: unknown) {
      setError(submitError instanceof Error ? submitError.message : "Could not add the course.");
    } finally {
      setIsSaving(false);
    }
  }

  return (
    <form className="space-y-4 rounded-2xl border border-ink/10 bg-white p-6 shadow-sm" onSubmit={handleSubmit}>
      <h3 className="text-lg font-bold">Add a course</h3>
      <div className="grid gap-4 sm:grid-cols-[0.8fr_1.2fr]">
        <label className="block">
          <span className="mb-2 block text-sm font-semibold">Course ID</span>
          <input
            className={`${inputClass} uppercase placeholder:normal-case`}
            maxLength={20}
            onChange={(event) => setForm({ ...form, courseId: event.target.value })}
            placeholder="CS455"
            required
            value={form.courseId}
          />
        </label>
        <label className="block">
          <span className="mb-2 block text-sm font-semibold">Course title</span>
          <input
            className={inputClass}
            maxLength={160}
            onChange={(event) => setForm({ ...form, title: event.target.value })}
            placeholder="Software Engineering"
            required
            value={form.title}
          />
        </label>
      </div>
      <label className="block">
        <span className="mb-2 block text-sm font-semibold">Instructor (optional)</span>
        <input
          className={inputClass}
          maxLength={120}
          onChange={(event) => setForm({ ...form, instructor: event.target.value })}
          placeholder="Prof. Rao"
          value={form.instructor}
        />
      </label>
      {error && <p className="rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{error}</p>}
      {message && <p className="rounded-lg bg-emerald-50 px-4 py-3 text-sm text-emerald-800" role="status">{message}</p>}
      <button
        className="rounded-lg bg-ocean px-5 py-3 font-semibold text-white hover:bg-ocean/90 disabled:opacity-60"
        disabled={isSaving}
        type="submit"
      >
        {isSaving ? "Adding…" : "Add course"}
      </button>
    </form>
  );
}

// Parses "CS455, Software Engineering, Prof. Rao" lines. Tabs also work, so a
// range copied from a spreadsheet can be pasted directly.
function parseCourseLines(text: string) {
  return text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const parts = line.split(line.includes("\t") ? "\t" : ",").map((part) => part.trim());
      return { courseId: parts[0] ?? "", title: parts[1] ?? "", instructor: parts.slice(2).join(", ") };
    });
}

function BulkAddForm({
  accessToken,
  existing,
  onAdded,
}: {
  accessToken: string;
  existing: CatalogCourse[];
  onAdded: (courses: CatalogCourse[]) => void;
}) {
  const [text, setText] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [summary, setSummary] = useState("");
  const [problems, setProblems] = useState<string[]>([]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const rows = parseCourseLines(text);
    if (rows.length === 0) return;
    setIsSaving(true);
    setSummary("");
    setProblems([]);

    const known = new Set(existing.map((course) => course.courseId));
    const added: CatalogCourse[] = [];
    const failed: string[] = [];
    const remaining: string[] = [];
    let skipped = 0;
    for (const row of rows) {
      const label = row.courseId || "(blank course ID)";
      if (known.has(row.courseId.trim().toUpperCase().replace(/\s+/g, " "))) {
        skipped += 1;
        continue;
      }
      try {
        const created = await createCourse(accessToken, row);
        added.push(created);
        known.add(created.courseId);
      } catch (error: unknown) {
        failed.push(`${label}: ${error instanceof Error ? error.message : "could not be added"}`);
        remaining.push([row.courseId, row.title, row.instructor].filter(Boolean).join(", "));
      }
    }

    onAdded(added);
    setSummary(
      `Added ${added.length} of ${rows.length}.` +
        (skipped ? ` ${skipped} already on the list.` : "") +
        (failed.length ? ` ${failed.length} need fixing (left in the box).` : ""),
    );
    setProblems(failed);
    setText(remaining.join("\n"));
    setIsSaving(false);
  }

  return (
    <form className="flex flex-col space-y-4 rounded-2xl border border-ink/10 bg-white p-6 shadow-sm" onSubmit={handleSubmit}>
      <h3 className="text-lg font-bold">Add several courses</h3>
      <label className="block flex-1">
        <span className="mb-2 block text-sm text-ink/65">
          One course per line: course ID, title, instructor (optional). You can also paste
          columns from a spreadsheet.
        </span>
        <textarea
          className={`${inputClass} min-h-36 font-mono text-sm`}
          onChange={(event) => setText(event.target.value)}
          placeholder={"CS455, Software Engineering, Prof. Rao\nMTH101, Mathematics I\nPHY102, Physics II"}
          value={text}
        />
      </label>
      {summary && (
        <p
          className={`rounded-lg px-4 py-3 text-sm ${problems.length ? "bg-amber-50 text-amber-900" : "bg-emerald-50 text-emerald-800"}`}
          role="status"
        >
          {summary}
        </p>
      )}
      {problems.length > 0 && (
        <ul className="space-y-1 text-sm text-rose-800">
          {problems.map((problem) => <li key={problem}>{problem}</li>)}
        </ul>
      )}
      <button
        className="self-start rounded-lg border border-ocean px-5 py-3 font-semibold text-ocean hover:bg-ocean/5 disabled:opacity-60"
        disabled={isSaving || !text.trim()}
        type="submit"
      >
        {isSaving ? "Adding courses…" : "Add all"}
      </button>
    </form>
  );
}

export default CourseCatalogSection;
