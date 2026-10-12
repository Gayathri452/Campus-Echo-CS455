import { useEffect, useState } from "react";
import type { TicketSummary } from "../../services/appealsShared";
import type { AuthSession } from "../../services/auth";
import { fetchMyTickets } from "../../services/appealsQueue";
import AppealRow from "./AppealRow";

export interface ActiveAppealsPageProps {
  session: AuthSession;
  onOpenTicket: (ticketId: string) => void;
}

function ActiveAppealsPage({ session, onOpenTicket }: ActiveAppealsPageProps) {
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;

    setIsLoading(true);
    void fetchMyTickets(session.accessToken)
      .then((result) => {
        if (!cancelled) {
          setTickets(result);
          setError("");
        }
      })
      .catch((requestError: unknown) => {
        if (!cancelled) {
          setError(
            requestError instanceof Error
              ? requestError.message
              : "Your active appeals could not be loaded.",
          );
        }
      })
      .finally(() => {
        if (!cancelled) {
          setIsLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [session.accessToken]);

  return (
    <div>
      <p className="text-sm font-bold uppercase tracking-[0.2em] text-ocean">
        Moderator
      </p>
      <h1 className="mt-3 text-3xl font-bold">Active appeals</h1>
      <p className="mt-3 max-w-2xl leading-7 text-ink/65">
        Continue reviewing tickets you have claimed.
      </p>

      {error && (
        <p
          className="mt-6 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800"
          role="alert"
        >
          {error}
        </p>
      )}

      {isLoading && (
        <p className="mt-6 text-sm text-ink/65" role="status">
          Loading active appeals…
        </p>
      )}

      {tickets.length > 0 && (
        <div className="mt-6 space-y-3">
          {tickets.map((ticket) => (
            <AppealRow
              key={ticket.id}
              onSelect={() => onOpenTicket(ticket.id)}
              ticket={ticket}
            />
          ))}
        </div>
      )}

      {!isLoading && !error && tickets.length === 0 && (
        <p className="mt-6 rounded-2xl border border-ink/10 bg-white p-6 text-ink/65 shadow-sm">
          You have no active appeals.
        </p>
      )}
    </div>
  );
}

export default ActiveAppealsPage;
