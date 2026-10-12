import { useState } from "react";
import type { AuthSession } from "../../services/auth";
import ActiveAppealsPage from "./ActiveAppealsPage";
import QueuePage from "./QueuePage";
import TicketDetailPage from "./TicketDetailPage";

// Shell for the three moderator pages. It owns navigation and the "successful"
// notice, so the pages never touch App.tsx or each other.
//
// The repo has no router and the access token lives in memory, so the current
// page is plain state. The props each page receives are the contract between
// the two moderator developers: keep them as they are.
//
//   QueuePage          (page 1, Person A) onTicketClaimed(id) after a claim
//   TicketDetailPage   (page 2, Person B) onDone(message) after release, publish
//                                         or delete, or if the ticket is gone
//   ActiveAppealsPage  (page 3, Person A) onOpenTicket(id) to reopen a ticket

type View =
  | { page: "queue" }
  | { page: "active" }
  | { page: "ticket"; ticketId: string };

interface ModeratorAppProps {
  session: AuthSession;
  onLogout: () => void;
}

function ModeratorApp({ session, onLogout }: ModeratorAppProps) {
  const [view, setView] = useState<View>({ page: "queue" });
  const [notice, setNotice] = useState("");

  function goTo(next: View) {
    setNotice("");
    setView(next);
  }

  // Page 2 calls this when it is finished: show the message, return to page 1.
  function finishTicket(message: string) {
    setNotice(message);
    setView({ page: "queue" });
  }

  function navClass(active: boolean) {
    return `rounded-lg px-4 py-2 text-sm font-semibold ${
      active ? "bg-ocean text-white" : "hover:bg-mist"
    }`;
  }

  return (
    <main className="min-h-screen bg-mist text-ink">
      <header className="border-b border-ink/10 bg-white">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 px-6 py-5">
          <span className="text-xl font-bold tracking-tight text-ocean">
            CampusEcho
          </span>
          <nav className="flex items-center gap-2" aria-label="Moderator">
            <button
              className={navClass(view.page === "queue")}
              onClick={() => goTo({ page: "queue" })}
              type="button"
            >
              Appeal queue
            </button>
            <button
              className={navClass(view.page === "active")}
              onClick={() => goTo({ page: "active" })}
              type="button"
            >
              Active appeals
            </button>
            <button
              className="rounded-lg border border-ink/15 px-4 py-2 text-sm font-semibold hover:bg-mist"
              onClick={onLogout}
              type="button"
            >
              Sign out
            </button>
          </nav>
        </div>
      </header>

      <section className="mx-auto max-w-5xl px-6 py-10">
        {notice && (
          <p
            className="mb-6 rounded-lg bg-emerald-50 px-4 py-3 text-sm text-emerald-800"
            role="status"
          >
            {notice}
          </p>
        )}

        {view.page === "queue" && (
          <QueuePage
            session={session}
            onTicketClaimed={(ticketId) => goTo({ page: "ticket", ticketId })}
          />
        )}
        {view.page === "active" && (
          <ActiveAppealsPage
            session={session}
            onOpenTicket={(ticketId) => goTo({ page: "ticket", ticketId })}
          />
        )}
        {view.page === "ticket" && (
          <TicketDetailPage
            key={view.ticketId}
            session={session}
            ticketId={view.ticketId}
            onDone={finishTicket}
          />
        )}
      </section>
    </main>
  );
}

export default ModeratorApp;
