import { useEffect, useState, type FormEvent } from "react";
import {
  createAnnouncement,
  deleteAnnouncement,
  listAllAnnouncements,
  type Announcement,
} from "../../services/announcements";
import { listCourses, type CatalogCourse } from "../../services/courses";

// Administrator view: post notices that students see on their Overview page.

interface AnnouncementsSectionProps {
  accessToken: string;
}

const inputClass =
  "w-full rounded-lg border border-ink/15 px-4 py-3 outline-none focus:border-ocean focus:ring-2 focus:ring-ocean/15";
const dateTime = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

function isExpired(item: Announcement) {
  return item.expiresAt !== undefined && new Date(item.expiresAt).getTime() <= Date.now();
}

function AnnouncementsSection({ accessToken }: AnnouncementsSectionProps) {
  const [items, setItems] = useState<Announcement[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [listMessage, setListMessage] = useState("");
  const [removingId, setRemovingId] = useState("");

  async function refresh() {
    setIsLoading(true);
    setLoadError("");
    try {
      setItems(await listAllAnnouncements(accessToken));
    } catch (error: unknown) {
      setLoadError(error instanceof Error ? error.message : "Could not load announcements.");
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, [accessToken]);

  async function handleRemove(item: Announcement) {
    if (!window.confirm(`Remove the announcement “${item.title}”? Students will no longer see it.`)) return;
    setRemovingId(item.id);
    setListMessage("");
    try {
      await deleteAnnouncement(accessToken, item.id);
      setItems((current) => current.filter((existing) => existing.id !== item.id));
      setListMessage(`“${item.title}” was removed.`);
    } catch (error: unknown) {
      setListMessage(error instanceof Error ? error.message : "Could not remove the announcement.");
    } finally {
      setRemovingId("");
    }
  }

  const active = items.filter((item) => !isExpired(item));
  const expired = items.filter(isExpired);

  return (
    <section className="mt-14 border-t border-ink/10 pt-10">
      <h2 className="text-2xl font-bold">Announcements</h2>
      <p className="mt-3 max-w-2xl leading-7 text-ink/65">
        Post a notice for students, such as a feedback window opening for a course. It appears on
        every student's Overview page until it expires or you remove it.
      </p>

      <div className="mt-8 grid gap-6 lg:grid-cols-[1fr_1.1fr]">
        <AnnouncementForm
          accessToken={accessToken}
          onPosted={(created) => {
            setItems((current) => [created, ...current]);
            setListMessage("");
          }}
        />

        <div className="rounded-2xl border border-ink/10 bg-white p-6 shadow-sm">
          <div className="flex items-center justify-between gap-3">
            <h3 className="text-lg font-bold">Showing to students ({active.length})</h3>
            <button
              className="rounded-lg border border-ink/15 px-3 py-1.5 text-sm font-semibold hover:bg-mist disabled:opacity-60"
              disabled={isLoading}
              onClick={() => void refresh()}
              type="button"
            >
              Refresh
            </button>
          </div>
          {loadError && <p className="mt-4 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{loadError}</p>}
          {listMessage && <p className="mt-4 rounded-lg bg-mist px-4 py-3 text-sm text-ink/75" role="status">{listMessage}</p>}
          {isLoading ? (
            <p className="mt-4 text-sm text-ink/60" role="status">Loading announcements…</p>
          ) : active.length === 0 ? (
            <p className="mt-4 text-sm text-ink/60">Nothing is showing to students right now.</p>
          ) : (
            <ul className="mt-4 space-y-3">
              {active.map((item) => (
                <AnnouncementRow
                  isRemoving={removingId === item.id}
                  item={item}
                  key={item.id}
                  onRemove={() => void handleRemove(item)}
                />
              ))}
            </ul>
          )}
          {expired.length > 0 && (
            <details className="mt-5">
              <summary className="cursor-pointer text-sm font-semibold text-ink/60">
                Expired ({expired.length})
              </summary>
              <ul className="mt-3 space-y-3 opacity-70">
                {expired.map((item) => (
                  <AnnouncementRow
                    isRemoving={removingId === item.id}
                    item={item}
                    key={item.id}
                    onRemove={() => void handleRemove(item)}
                  />
                ))}
              </ul>
            </details>
          )}
        </div>
      </div>
    </section>
  );
}

function AnnouncementRow({ item, isRemoving, onRemove }: { item: Announcement; isRemoving: boolean; onRemove: () => void }) {
  return (
    <li className="rounded-xl border border-ink/10 p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-semibold">
            {item.pinned && <span className="mr-2 rounded-full bg-ocean/10 px-2 py-0.5 text-xs font-semibold text-ocean">Pinned</span>}
            {item.title}
          </p>
          <p className="mt-1 whitespace-pre-line text-sm leading-6 text-ink/70">{item.message}</p>
          <p className="mt-2 text-xs text-ink/50">
            {item.courseId ? `${item.courseId}. ` : "All courses. "}
            Posted {dateTime.format(new Date(item.createdAt))}
            {item.expiresAt ? `, ${isExpired(item) ? "expired" : "until"} ${dateTime.format(new Date(item.expiresAt))}` : ", no expiry"}
          </p>
        </div>
        <button
          className="shrink-0 rounded-lg px-3 py-1.5 text-sm font-semibold text-rose-700 hover:bg-rose-50 disabled:opacity-60"
          disabled={isRemoving}
          onClick={onRemove}
          type="button"
        >
          {isRemoving ? "Removing…" : "Remove"}
        </button>
      </div>
    </li>
  );
}

const emptyForm = { title: "", message: "", courseId: "", pinned: false, expiresOn: "" };

function AnnouncementForm({ accessToken, onPosted }: { accessToken: string; onPosted: (item: Announcement) => void }) {
  const [form, setForm] = useState(emptyForm);
  const [courses, setCourses] = useState<CatalogCourse[]>([]);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [isPosting, setIsPosting] = useState(false);

  useEffect(() => {
    listCourses(accessToken).then(setCourses).catch(() => setCourses([]));
  }, [accessToken]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setMessage("");
    setIsPosting(true);
    try {
      // A date picker gives a day; the announcement lasts until the end of it.
      const expiresAt = form.expiresOn ? new Date(`${form.expiresOn}T23:59:59`).toISOString() : undefined;
      const created = await createAnnouncement(accessToken, {
        title: form.title,
        message: form.message,
        courseId: form.courseId || undefined,
        pinned: form.pinned,
        expiresAt,
      });
      onPosted(created);
      setMessage(`“${created.title}” is now showing to students.`);
      setForm(emptyForm);
    } catch (submitError: unknown) {
      setError(submitError instanceof Error ? submitError.message : "Could not post the announcement.");
    } finally {
      setIsPosting(false);
    }
  }

  const today = new Date();
  const minDate = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, "0")}-${String(today.getDate()).padStart(2, "0")}`;

  return (
    <form className="space-y-4 rounded-2xl border border-ink/10 bg-white p-6 shadow-sm" onSubmit={handleSubmit}>
      <h3 className="text-lg font-bold">New announcement</h3>
      <label className="block">
        <span className="mb-2 block text-sm font-semibold">Title</span>
        <input
          className={inputClass}
          maxLength={120}
          onChange={(event) => setForm({ ...form, title: event.target.value })}
          placeholder="Mid-semester feedback is open"
          required
          value={form.title}
        />
      </label>
      <label className="block">
        <span className="mb-2 block text-sm font-semibold">Message</span>
        <textarea
          className={`${inputClass} min-h-28`}
          maxLength={1000}
          onChange={(event) => setForm({ ...form, message: event.target.value })}
          placeholder="Share honest, specific feedback about your courses before Friday. Reviews are anonymous."
          required
          value={form.message}
        />
        <span className="mt-1 block text-right text-xs text-ink/45">{form.message.length}/1000</span>
      </label>
      <div className="grid gap-4 sm:grid-cols-2">
        <label className="block">
          <span className="mb-2 block text-sm font-semibold">Course (optional)</span>
          <select
            className={inputClass}
            onChange={(event) => setForm({ ...form, courseId: event.target.value })}
            value={form.courseId}
          >
            <option value="">All courses</option>
            {courses.map((course) => (
              <option key={course.courseId} value={course.courseId}>
                {course.courseId} — {course.title}
              </option>
            ))}
          </select>
        </label>
        <label className="block">
          <span className="mb-2 block text-sm font-semibold">Show until (optional)</span>
          <input
            className={inputClass}
            min={minDate}
            onChange={(event) => setForm({ ...form, expiresOn: event.target.value })}
            type="date"
            value={form.expiresOn}
          />
        </label>
      </div>
      <label className="flex items-center gap-3 text-sm font-semibold">
        <input
          checked={form.pinned}
          className="h-4 w-4 accent-[#176b67]"
          onChange={(event) => setForm({ ...form, pinned: event.target.checked })}
          type="checkbox"
        />
        Pin to the top
      </label>
      {error && <p className="rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{error}</p>}
      {message && <p className="rounded-lg bg-emerald-50 px-4 py-3 text-sm text-emerald-800" role="status">{message}</p>}
      <button
        className="rounded-lg bg-ocean px-5 py-3 font-semibold text-white hover:bg-ocean/90 disabled:opacity-60"
        disabled={isPosting}
        type="submit"
      >
        {isPosting ? "Posting…" : "Post announcement"}
      </button>
    </form>
  );
}

export default AnnouncementsSection;
