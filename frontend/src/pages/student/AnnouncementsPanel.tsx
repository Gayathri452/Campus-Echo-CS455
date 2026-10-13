import { useEffect, useState } from "react";
import { listAnnouncements, type Announcement } from "../../services/announcements";

// Administrator announcements on the student Overview page. Students can hide
// an announcement; that choice is remembered in this browser only.

interface AnnouncementsPanelProps {
  accessToken?: string;
  ownerId: string;
}

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" });
const dismissedPrefix = "campusecho.dismissedAnnouncements.v1:";

function loadDismissed(ownerId: string): string[] {
  try {
    const parsed: unknown = JSON.parse(window.localStorage.getItem(dismissedPrefix + ownerId) ?? "[]");
    return Array.isArray(parsed) ? parsed.filter((id): id is string => typeof id === "string") : [];
  } catch {
    return [];
  }
}

function saveDismissed(ownerId: string, ids: string[]) {
  try {
    window.localStorage.setItem(dismissedPrefix + ownerId, JSON.stringify(ids.slice(-200)));
  } catch {
    // Hiding still works for this visit if storage is blocked.
  }
}

function AnnouncementsPanel({ accessToken, ownerId }: AnnouncementsPanelProps) {
  const [items, setItems] = useState<Announcement[]>([]);
  const [dismissed, setDismissed] = useState<string[]>(() => loadDismissed(ownerId));
  const [showHidden, setShowHidden] = useState(false);

  useEffect(() => {
    if (!accessToken) return;
    let cancelled = false;
    listAnnouncements(accessToken)
      .then((result) => {
        if (!cancelled) setItems(result);
      })
      .catch(() => {
        // Announcements are optional; the dashboard works without them.
      });
    return () => {
      cancelled = true;
    };
  }, [accessToken]);

  function hide(id: string) {
    const next = [...dismissed.filter((existing) => existing !== id), id];
    setDismissed(next);
    saveDismissed(ownerId, next);
  }

  function showAgain(id: string) {
    const next = dismissed.filter((existing) => existing !== id);
    setDismissed(next);
    saveDismissed(ownerId, next);
  }

  const visible = items.filter((item) => !dismissed.includes(item.id));
  const hidden = items.filter((item) => dismissed.includes(item.id));
  if (items.length === 0) return null;

  return (
    <section aria-label="Announcements" className="mb-10">
      {visible.length > 0 && (
        <ul className="space-y-3">
          {visible.map((item) => (
            <li
              className={`rounded-xl border bg-white p-5 shadow-sm ${item.pinned ? "border-ocean/40 border-l-4 border-l-ocean" : "border-ink/10"}`}
              key={item.id}
            >
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <p className="text-xs font-semibold text-ocean">
                    {item.pinned ? "Pinned announcement" : "Announcement"}
                    {item.courseId ? ` for ${item.courseId}` : ""}
                  </p>
                  <h2 className="mt-1 text-lg font-bold">{item.title}</h2>
                  <p className="mt-2 whitespace-pre-line text-sm leading-6 text-ink/70">{item.message}</p>
                  <p className="mt-3 text-xs text-ink/45">
                    Posted {dateFormat.format(new Date(item.createdAt))}
                    {item.expiresAt ? `. Open until ${dateFormat.format(new Date(item.expiresAt))}` : ""}
                  </p>
                </div>
                <button
                  aria-label={`Hide announcement: ${item.title}`}
                  className="shrink-0 rounded-lg px-3 py-1.5 text-sm font-semibold text-ink/55 hover:bg-mist"
                  onClick={() => hide(item.id)}
                  type="button"
                >
                  Hide
                </button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {hidden.length > 0 && (
        <div className="mt-3 text-sm text-ink/55">
          <button className="font-semibold hover:text-ocean" onClick={() => setShowHidden(!showHidden)} type="button">
            {showHidden ? "Close hidden announcements" : `Show ${hidden.length} hidden ${hidden.length === 1 ? "announcement" : "announcements"}`}
          </button>
          {showHidden && (
            <ul className="mt-2 space-y-1">
              {hidden.map((item) => (
                <li className="flex items-center gap-3" key={item.id}>
                  <span>{item.title}</span>
                  <button className="font-semibold text-ocean hover:underline" onClick={() => showAgain(item.id)} type="button">
                    Show again
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  );
}

export default AnnouncementsPanel;
