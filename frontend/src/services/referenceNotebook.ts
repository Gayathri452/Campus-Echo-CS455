// "My reference codes" notebook.
//
// Codes are stored only in this browser (localStorage), under a key for the
// signed-in student. They are deliberately NOT sent to the server: if the
// server stored "student X owns code Y" it could link students to their
// anonymous reviews, which the anonymity requirement (NFR-1) forbids.

export interface NotebookEntry {
  referenceCode: string;
  courseId: string;
  courseTitle: string;
  note: string;
  savedAt: string;
  lastStatus?: string;
  flagReason?: string;
  appealStatus?: string;
  resolutionComment?: string;
  checkedAt?: string;
}

const storagePrefix = "campusecho.referenceNotebook.v1:";
const maxEntries = 200;
const maxNoteLength = 2000;

function storageKey(ownerId: string) {
  return storagePrefix + ownerId;
}

export function normalizeReferenceCode(code: string) {
  return code.trim().toUpperCase().replace(/\s+/g, "");
}

function isEntry(value: unknown): value is NotebookEntry {
  if (typeof value !== "object" || value === null) return false;
  const entry = value as Record<string, unknown>;
  return (
    typeof entry.referenceCode === "string" &&
    typeof entry.courseId === "string" &&
    typeof entry.courseTitle === "string" &&
    typeof entry.note === "string" &&
    typeof entry.savedAt === "string"
  );
}

/** Returns saved entries, newest first. Never throws. */
export function loadNotebook(ownerId: string): NotebookEntry[] {
  try {
    const raw = window.localStorage.getItem(storageKey(ownerId));
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed.filter(isEntry) : [];
  } catch {
    return [];
  }
}

/** Saves entries. Returns false when the browser blocks storage. */
export function saveNotebook(ownerId: string, entries: NotebookEntry[]): boolean {
  try {
    window.localStorage.setItem(storageKey(ownerId), JSON.stringify(entries.slice(0, maxEntries)));
    return true;
  } catch {
    return false;
  }
}

/**
 * Adds a code (or refreshes its course details if it is already saved) and
 * returns the new list, or null when storage is unavailable.
 */
export function addToNotebook(
  ownerId: string,
  entry: { referenceCode: string; courseId?: string; courseTitle?: string; note?: string; lastStatus?: string },
): NotebookEntry[] | null {
  const referenceCode = normalizeReferenceCode(entry.referenceCode);
  const entries = loadNotebook(ownerId);
  const existing = entries.find((item) => item.referenceCode === referenceCode);
  if (existing) {
    existing.courseId = existing.courseId || entry.courseId?.trim() || "";
    existing.courseTitle = existing.courseTitle || entry.courseTitle?.trim() || "";
  } else {
    entries.unshift({
      referenceCode,
      courseId: entry.courseId?.trim() ?? "",
      courseTitle: entry.courseTitle?.trim() ?? "",
      note: (entry.note ?? "").slice(0, maxNoteLength),
      savedAt: new Date().toISOString(),
      lastStatus: entry.lastStatus,
      checkedAt: entry.lastStatus ? new Date().toISOString() : undefined,
    });
  }
  return saveNotebook(ownerId, entries) ? entries : null;
}

export function updateNotebookEntry(
  ownerId: string,
  referenceCode: string,
  changes: Partial<Omit<NotebookEntry, "referenceCode" | "savedAt">>,
): NotebookEntry[] {
  const entries = loadNotebook(ownerId).map((entry) =>
    entry.referenceCode === referenceCode
      ? { ...entry, ...changes, note: (changes.note ?? entry.note).slice(0, maxNoteLength) }
      : entry,
  );
  saveNotebook(ownerId, entries);
  return entries;
}

export function removeFromNotebook(ownerId: string, referenceCode: string): NotebookEntry[] {
  const entries = loadNotebook(ownerId).filter((entry) => entry.referenceCode !== referenceCode);
  saveNotebook(ownerId, entries);
  return entries;
}

export function clearNotebook(ownerId: string) {
  try {
    window.localStorage.removeItem(storageKey(ownerId));
  } catch {
    // Nothing to clear if storage is unavailable.
  }
}

/** Plain-text backup a student can keep or move to another device. */
export function notebookAsText(entries: NotebookEntry[]) {
  const lines = ["CampusEcho reference codes", `Exported ${new Date().toLocaleString()}`, ""];
  for (const entry of entries) {
    lines.push(entry.referenceCode);
    if (entry.courseId || entry.courseTitle) {
      lines.push(`  Course: ${[entry.courseId, entry.courseTitle].filter(Boolean).join(" — ")}`);
    }
    lines.push(`  Saved: ${new Date(entry.savedAt).toLocaleString()}`);
    if (entry.lastStatus) lines.push(`  Last status: ${entry.lastStatus}`);
    if (entry.note.trim()) lines.push(`  Note: ${entry.note.trim().replace(/\n/g, "\n        ")}`);
    lines.push("");
  }
  return lines.join("\n");
}

// The submit form only has the access token, so the notebook finds the
// signed-in student's ID through GET /api/v1/auth/me (cached per token).
let ownerCache: { token: string; promise: Promise<string> } | null = null;

export function getNotebookOwner(accessToken: string): Promise<string> {
  if (ownerCache?.token === accessToken) return ownerCache.promise;
  const promise = fetch("/api/v1/auth/me", {
    headers: { Authorization: `Bearer ${accessToken}` },
    credentials: "same-origin",
  })
    .then(async (response) => {
      const user: unknown = await response.json().catch(() => null);
      if (!response.ok || typeof user !== "object" || user === null || typeof (user as { id?: unknown }).id !== "string") {
        throw new Error("Could not identify the signed-in student.");
      }
      return (user as { id: string }).id;
    })
    .catch((error: unknown) => {
      ownerCache = null;
      throw error;
    });
  ownerCache = { token: accessToken, promise };
  return promise;
}
