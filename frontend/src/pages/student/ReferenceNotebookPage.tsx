import { useEffect, useRef, useState, type FormEvent } from "react";
import { getReviewStatus } from "../../services/feedback";
import {
  addToNotebook,
  clearNotebook,
  loadNotebook,
  normalizeReferenceCode,
  notebookAsText,
  removeFromNotebook,
  updateNotebookEntry,
  type NotebookEntry,
} from "../../services/referenceNotebook";
import { COURSE_LIST_ID } from "./CourseSuggestions";

// "My codes": the student's private notebook of review reference codes, with
// a note per code and the latest status. Stored only in this browser.

interface ReferenceNotebookPageProps {
  ownerId: string;
  accessToken?: string;
  /** Opens the Track status page (the code is copied first). */
  onTrack: () => void;
}

const statusLabel: Record<string, string> = {
  submitted: "Submitted",
  flagged: "Flagged for review",
  rejected: "Rejected",
  published: "Published",
  deleted: "Removed",
  triage_failed: "Moderation needs attention",
};

const statusTone: Record<string, string> = {
  published: "bg-emerald-50 text-emerald-800",
  flagged: "bg-amber-50 text-amber-900",
  rejected: "bg-rose-50 text-rose-800",
  deleted: "bg-rose-50 text-rose-800",
  triage_failed: "bg-rose-50 text-rose-800",
};

const appealLabel: Record<string, string> = {
  under_appeal: "Appeal waiting for a moderator",
  locked: "A moderator is reviewing the appeal",
  restored: "Published after appeal",
  deleted: "Removed after appeal",
};

const dateTime = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });
const autoCheckAfterMs = 5 * 60 * 1000;

function ReferenceNotebookPage({ ownerId, accessToken, onTrack }: ReferenceNotebookPageProps) {
  const [entries, setEntries] = useState<NotebookEntry[]>(() => loadNotebook(ownerId));
  const [checking, setChecking] = useState<Set<string>>(new Set());
  const [checkErrors, setCheckErrors] = useState<Record<string, string>>({});
  const [notice, setNotice] = useState("");
  const [query, setQuery] = useState("");
  const autoChecked = useRef(false);

  async function checkStatus(referenceCode: string) {
    setChecking((current) => new Set(current).add(referenceCode));
    setCheckErrors((current) => {
      const next = { ...current };
      delete next[referenceCode];
      return next;
    });
    try {
      const review = await getReviewStatus(referenceCode, accessToken);
      setEntries(
        updateNotebookEntry(ownerId, referenceCode, {
          courseId: review.courseId,
          courseTitle: review.courseTitle,
          lastStatus: review.status,
          flagReason: review.flagReason ?? "",
          appealStatus: review.appealStatus ?? "",
          resolutionComment: review.appealResolutionComment ?? "",
          checkedAt: new Date().toISOString(),
        }),
      );
    } catch (error: unknown) {
      setCheckErrors((current) => ({
        ...current,
        [referenceCode]: error instanceof Error ? error.message : "Could not check this code.",
      }));
    } finally {
      setChecking((current) => {
        const next = new Set(current);
        next.delete(referenceCode);
        return next;
      });
    }
  }

  async function checkAll(onlyStale: boolean) {
    const now = Date.now();
    const due = loadNotebook(ownerId).filter(
      (entry) => !onlyStale || !entry.checkedAt || now - new Date(entry.checkedAt).getTime() > autoCheckAfterMs,
    );
    for (const entry of due.slice(0, 25)) {
      await checkStatus(entry.referenceCode);
    }
  }

  // Refresh codes that have not been checked recently when the page opens.
  useEffect(() => {
    if (!autoChecked.current && accessToken) {
      autoChecked.current = true;
      void checkAll(true);
    }
    // Runs once per visit.
  }, [accessToken]);

  async function copy(text: string, message: string) {
    try {
      await navigator.clipboard.writeText(text);
      setNotice(message);
    } catch {
      setNotice("Copying is blocked in this browser. Select the code and copy it manually.");
    }
  }

  async function track(referenceCode: string) {
    await copy(referenceCode, "");
    onTrack();
  }

  function remove(entry: NotebookEntry) {
    if (window.confirm(`Remove ${entry.referenceCode} from My codes? You will not be able to look this review up without the code.`)) {
      setEntries(removeFromNotebook(ownerId, entry.referenceCode));
    }
  }

  function removeAll() {
    if (window.confirm("Remove every saved code from this device? Export them first if you may need them.")) {
      clearNotebook(ownerId);
      setEntries([]);
    }
  }

  function exportText() {
    const blob = new Blob([notebookAsText(entries)], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "campusecho-reference-codes.txt";
    link.click();
    URL.revokeObjectURL(url);
  }

  const needle = query.trim().toLowerCase();
  const visible = entries.filter(
    (entry) =>
      !needle ||
      entry.referenceCode.toLowerCase().includes(needle) ||
      entry.courseId.toLowerCase().includes(needle) ||
      entry.courseTitle.toLowerCase().includes(needle) ||
      entry.note.toLowerCase().includes(needle),
  );

  return (
    <section>
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-sm font-bold uppercase tracking-[0.2em] text-ocean">Private notebook</p>
          <h1 className="mt-3 text-3xl font-bold">My reference codes</h1>
          <p className="mt-3 max-w-2xl leading-7 text-ink/65">
            Codes are saved here automatically when you submit a review. They are kept only in
            this browser on this device and are never linked to your account on the server, so
            your reviews stay anonymous.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            className="rounded-lg bg-ocean px-4 py-2 text-sm font-semibold text-white hover:bg-ocean/90 disabled:opacity-60"
            disabled={entries.length === 0 || checking.size > 0}
            onClick={() => void checkAll(false)}
            type="button"
          >
            {checking.size > 0 ? "Checking…" : "Check all statuses"}
          </button>
          <button
            className="rounded-lg border border-ink/15 px-4 py-2 text-sm font-semibold hover:bg-white disabled:opacity-60"
            disabled={entries.length === 0}
            onClick={exportText}
            type="button"
          >
            Download backup
          </button>
        </div>
      </div>

      <AddCodeForm
        onAdd={(code, courseId, note) => {
          const updated = addToNotebook(ownerId, { referenceCode: code, courseId, note });
          if (updated) {
            setEntries(updated);
            setNotice(`${normalizeReferenceCode(code)} was added.`);
            void checkStatus(normalizeReferenceCode(code));
          } else {
            setNotice("This browser is blocking storage, so the code could not be saved.");
          }
        }}
      />

      {notice && (
        <p className="mt-5 rounded-lg bg-mist px-4 py-3 text-sm text-ink/75" role="status">
          {notice}
        </p>
      )}

      {entries.length === 0 ? (
        <div className="mt-6 rounded-xl border border-ink/10 bg-white p-6 text-ink/65 shadow-sm">
          <h2 className="font-bold text-ink">No saved codes on this device</h2>
          <p className="mt-2 max-w-2xl text-sm leading-6">
            Submit a review and its reference code will appear here. If you saved a code
            somewhere else, add it with the form above.
          </p>
        </div>
      ) : (
        <>
          {entries.length > 3 && (
            <div className="mt-6">
              <label className="sr-only" htmlFor="notebook-search">Search saved codes</label>
              <input
                className="input max-w-md"
                id="notebook-search"
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search codes, courses or notes"
                value={query}
              />
            </div>
          )}
          <ul className="mt-6 space-y-4">
            {visible.map((entry) => (
              <NotebookCard
                checkError={checkErrors[entry.referenceCode]}
                entry={entry}
                isChecking={checking.has(entry.referenceCode)}
                key={entry.referenceCode}
                onCheck={() => void checkStatus(entry.referenceCode)}
                onCopy={() => void copy(entry.referenceCode, `${entry.referenceCode} copied.`)}
                onNoteChange={(note) => setEntries(updateNotebookEntry(ownerId, entry.referenceCode, { note }))}
                onRemove={() => remove(entry)}
                onTrack={() => void track(entry.referenceCode)}
              />
            ))}
          </ul>
          {visible.length === 0 && <p className="mt-6 text-sm text-ink/60">No saved code matches “{query}”.</p>}
          <button
            className="mt-8 text-sm font-semibold text-rose-700 hover:underline"
            onClick={removeAll}
            type="button"
          >
            Remove all codes from this device
          </button>
        </>
      )}
    </section>
  );
}

function NotebookCard({
  entry,
  isChecking,
  checkError,
  onCheck,
  onCopy,
  onTrack,
  onRemove,
  onNoteChange,
}: {
  entry: NotebookEntry;
  isChecking: boolean;
  checkError?: string;
  onCheck: () => void;
  onCopy: () => void;
  onTrack: () => void;
  onRemove: () => void;
  onNoteChange: (note: string) => void;
}) {
  const [note, setNote] = useState(entry.note);
  const status = entry.lastStatus ?? "";
  const canAppeal = (status === "flagged" || status === "rejected") && !entry.appealStatus;

  return (
    <li className="rounded-xl border border-ink/10 bg-white p-5 shadow-sm">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="break-all font-mono text-lg font-bold tracking-wide text-ocean">{entry.referenceCode}</span>
            <button
              className="rounded-md px-2 py-1 text-xs font-semibold text-ink/60 hover:bg-mist"
              onClick={onCopy}
              type="button"
            >
              Copy
            </button>
          </div>
          <p className="mt-1 text-sm text-ink/70">
            {entry.courseId || entry.courseTitle
              ? [entry.courseId, entry.courseTitle].filter(Boolean).join(" — ")
              : "Course not recorded"}
          </p>
          <p className="mt-1 text-xs text-ink/45">Saved {dateTime.format(new Date(entry.savedAt))}</p>
        </div>
        <div className="sm:text-right">
          {status ? (
            <span className={`inline-block rounded-full px-3 py-1 text-xs font-semibold ${statusTone[status] ?? "bg-mist text-ink/70"}`}>
              {statusLabel[status] ?? status}
            </span>
          ) : (
            <span className="inline-block rounded-full bg-mist px-3 py-1 text-xs font-semibold text-ink/55">Not checked yet</span>
          )}
          {entry.checkedAt && (
            <p className="mt-1 text-xs text-ink/45">Checked {dateTime.format(new Date(entry.checkedAt))}</p>
          )}
        </div>
      </div>

      {(entry.flagReason || entry.appealStatus || entry.resolutionComment) && (
        <div className="mt-4 space-y-1 rounded-lg bg-mist/70 px-4 py-3 text-sm text-ink/70">
          {entry.flagReason && <p><span className="font-semibold">Flag reason:</span> {entry.flagReason}</p>}
          {entry.appealStatus && <p><span className="font-semibold">Appeal:</span> {appealLabel[entry.appealStatus] ?? entry.appealStatus}</p>}
          {entry.resolutionComment && <p><span className="font-semibold">Moderator note:</span> {entry.resolutionComment}</p>}
        </div>
      )}
      {checkError && (
        <p className="mt-4 rounded-lg bg-rose-50 px-4 py-2 text-sm text-rose-800" role="alert">{checkError}</p>
      )}

      <label className="mt-4 block">
        <span className="mb-1 block text-xs font-semibold text-ink/55">My note (only on this device)</span>
        <textarea
          className="input min-h-20 resize-y text-sm"
          maxLength={2000}
          onBlur={() => note !== entry.note && onNoteChange(note)}
          onChange={(event) => setNote(event.target.value)}
          placeholder="For example: what you wrote about, or when to check back"
          value={note}
        />
      </label>

      <div className="mt-4 flex flex-wrap gap-2">
        <button
          className="rounded-lg border border-ink/15 px-3 py-2 text-sm font-semibold hover:bg-mist disabled:opacity-60"
          disabled={isChecking}
          onClick={onCheck}
          type="button"
        >
          {isChecking ? "Checking…" : "Check status"}
        </button>
        <button
          className={`rounded-lg px-3 py-2 text-sm font-semibold ${canAppeal ? "bg-ocean text-white hover:bg-ocean/90" : "border border-ink/15 hover:bg-mist"}`}
          onClick={onTrack}
          title="Copies the code and opens Track status"
          type="button"
        >
          {canAppeal ? "Copy code and appeal" : "Copy code and open Track status"}
        </button>
        <button
          className="ml-auto rounded-lg px-3 py-2 text-sm font-semibold text-rose-700 hover:bg-rose-50"
          onClick={onRemove}
          type="button"
        >
          Remove
        </button>
      </div>
    </li>
  );
}

function AddCodeForm({ onAdd }: { onAdd: (code: string, courseId: string, note: string) => void }) {
  const [code, setCode] = useState("");
  const [courseId, setCourseId] = useState("");
  const [error, setError] = useState("");

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = normalizeReferenceCode(code);
    if (!/^CE-[A-Z0-9]{6,32}$/.test(normalized)) {
      setError("Reference codes look like CE-ABCD1234EFGH.");
      return;
    }
    setError("");
    onAdd(normalized, courseId.trim().toUpperCase(), "");
    setCode("");
    setCourseId("");
  }

  return (
    <details className="mt-6 rounded-xl border border-ink/10 bg-white p-5 shadow-sm">
      <summary className="cursor-pointer text-sm font-semibold">Add a code you saved somewhere else</summary>
      <form className="mt-4 grid gap-3 sm:grid-cols-[1.4fr_1fr_auto]" onSubmit={handleSubmit}>
        <label className="block">
          <span className="sr-only">Reference code</span>
          <input
            className="input font-mono uppercase placeholder:normal-case"
            onChange={(event) => setCode(event.target.value)}
            placeholder="Reference code, e.g. CE-ABCD1234EFGH"
            value={code}
          />
        </label>
        <label className="block">
          <span className="sr-only">Course ID (optional)</span>
          <input
            autoComplete="off"
            className="input uppercase placeholder:normal-case"
            list={COURSE_LIST_ID}
            onChange={(event) => setCourseId(event.target.value)}
            placeholder="Course ID (optional)"
            value={courseId}
          />
        </label>
        <button className="rounded-lg bg-ocean px-4 py-2 text-sm font-semibold text-white hover:bg-ocean/90" type="submit">
          Save code
        </button>
        {error && <p className="text-sm text-rose-800 sm:col-span-3" role="alert">{error}</p>}
      </form>
    </details>
  );
}

export default ReferenceNotebookPage;
