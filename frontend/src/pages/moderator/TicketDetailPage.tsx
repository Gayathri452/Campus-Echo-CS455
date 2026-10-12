import { useEffect, useState } from "react";
import { ApiError, type TicketDetail } from "../../services/appealsShared";
import type { AuthSession } from "../../services/auth";
import {
  deleteTicket,
  fetchTicket,
  releaseTicket,
  restoreTicket,
} from "../../services/appealsResolve";
import DeleteModal from "./DeleteModal";

// Person B owns this file (page 2, SCRUM-12 and SCRUM-13). Keep the props.
//
// Load the ticket with fetchTicket from services/appealsResolve.ts and show:
// course id, year and professor; the reference code; the feedback; the reason
// the LLM flagged it; the student's appeal comment. Bottom right, three
// buttons: send back to queue (releaseTicket), publish (restoreTicket) and
// delete (opens a DeleteModal with Cancel and Submit; the comment is required).
// After any of the three succeeds, call onDone with a short success message,
// for example onDone("Review published"). The shell shows it on page 1.
//
// If fetchTicket fails with a 403 or 404 ApiError the lock has expired or the
// ticket was taken: call onDone("This appeal is no longer assigned to you").
// Put DeleteModal in its own file, src/pages/moderator/DeleteModal.tsx.

export interface TicketDetailPageProps {
  session: AuthSession;
  ticketId: string;
  onDone: (message: string) => void;
}

function TicketDetailPage({ session, ticketId, onDone }: TicketDetailPageProps) {
  const [ticket, setTicket] = useState<TicketDetail | null>(null);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isDeleteOpen, setIsDeleteOpen] = useState(false);

  async function finishAction(
    action: () => Promise<void>,
    message: string,
  ) {
    setError("");
    setIsSubmitting(true);
    try {
      await action();
      onDone(message);
    } catch (requestError: unknown) {
      if (
        requestError instanceof ApiError &&
        (requestError.status === 403 || requestError.status === 404)
      ) {
        onDone("This appeal is no longer assigned to you");
        return;
      }
      setError(
        requestError instanceof Error
          ? requestError.message
          : "Could not complete this action.",
      );
      setIsSubmitting(false);
    }
  }

  useEffect(() => {
    let cancelled = false;

    fetchTicket(session.accessToken, ticketId)
      .then((result) => {
        if (!cancelled) setTicket(result);
      })
      .catch((requestError: unknown) => {
        if (cancelled) return;
        if (
          requestError instanceof ApiError &&
          (requestError.status === 403 || requestError.status === 404)
        ) {
          onDone("This appeal is no longer assigned to you");
          return;
        }
        setError(
          requestError instanceof Error
            ? requestError.message
            : "Could not load this appeal.",
        );
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [onDone, session.accessToken, ticketId]);

  if (isLoading) {
    return <p role="status">Loading appeal details...</p>;
  }

  if (error) {
    return (
      <p className="rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">
        {error}
      </p>
    );
  }

  if (!ticket) return null;

  return (
    <article className="rounded-xl border border-ink/10 bg-white p-6 shadow-sm">
      <p className="text-sm font-semibold text-ocean">{ticket.referenceCode}</p>
      <h1 className="mt-2 text-2xl font-bold">Appeal details</h1>
      <p className="mt-2 text-ink/65">
        {ticket.courseId} · {ticket.year} · {ticket.professor}
      </p>

      <dl className="mt-8 grid gap-6">
        <div>
          <dt className="text-sm font-semibold text-ink/60">Review</dt>
          <dd className="mt-2 whitespace-pre-wrap leading-7">{ticket.feedbackText}</dd>
        </div>
        <div>
          <dt className="text-sm font-semibold text-ink/60">Flag reason</dt>
          <dd className="mt-2 leading-7">{ticket.flagReason}</dd>
        </div>
        <div>
          <dt className="text-sm font-semibold text-ink/60">Appeal comment</dt>
          <dd className="mt-2 whitespace-pre-wrap leading-7">{ticket.appealComment}</dd>
        </div>
      </dl>

      {error && (
        <p className="mt-6 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">
          {error}
        </p>
      )}

      <div className="mt-8 flex flex-wrap justify-end gap-3 border-t border-ink/10 pt-6">
        <button
          className="rounded-lg border border-ink/15 px-4 py-2 text-sm font-semibold hover:bg-mist disabled:opacity-60"
          disabled={isSubmitting}
          onClick={() =>
            void finishAction(
              () => releaseTicket(session.accessToken, ticketId),
              "Appeal sent back to the queue",
            )
          }
          type="button"
        >
          Send back to queue
        </button>
        <button
          className="rounded-lg border border-rose-700 px-4 py-2 text-sm font-semibold text-rose-800 hover:bg-rose-50 disabled:opacity-60"
          disabled={isSubmitting}
          onClick={() => setIsDeleteOpen(true)}
          type="button"
        >
          Delete review
        </button>
        <button
          className="rounded-lg bg-ocean px-4 py-2 text-sm font-semibold text-white hover:bg-ocean/90 disabled:opacity-60"
          disabled={isSubmitting}
          onClick={() =>
            void finishAction(
              () => restoreTicket(session.accessToken, ticketId),
              "Review published",
            )
          }
          type="button"
        >
          Publish review
        </button>
      </div>

      {isDeleteOpen && (
        <DeleteModal
          isSubmitting={isSubmitting}
          onCancel={() => setIsDeleteOpen(false)}
          onSubmit={(comment) =>
            void finishAction(
              () => deleteTicket(session.accessToken, ticketId, comment),
              "Review deleted",
            )
          }
        />
      )}
    </article>
  );
}

export default TicketDetailPage;
