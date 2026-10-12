import { useEffect, useState } from "react";
import { ApiError, type SortOrder, type TicketSummary } from "../../services/appealsShared";
import type { AuthSession } from "../../services/auth";
import { claimTicket, fetchQueue } from "../../services/appealsQueue";
import AppealRow from "./AppealRow";

export interface QueuePageProps {
  session: AuthSession;
  onTicketClaimed: (ticketId: string) => void;
}

function QueuePage({ session, onTicketClaimed }: QueuePageProps) {
  const [sort, setSort] = useState<SortOrder>("asc");
  const [tickets, setTickets] = useState<TicketSummary[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isClaiming, setIsClaiming] = useState(false);
  const [claimError, setClaimError] = useState("");
  const [listError, setListError] = useState("");
  const [refreshVersion, setRefreshVersion] = useState(0);

  useEffect(() => {
    let cancelled = false;

    setIsLoading(true);
    void fetchQueue(session.accessToken, sort)
      .then((result) => {
        if (!cancelled) {
          setTickets(result);
          setListError("");
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setListError(
            error instanceof Error
              ? error.message
              : "The appeal queue could not be loaded.",
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
  }, [refreshVersion, session.accessToken, sort]);

  async function handleClaim(ticketId: string) {
    if (isClaiming) {
      return;
    }

    setIsClaiming(true);
    setClaimError("");
    try {
      await claimTicket(session.accessToken, ticketId);
      onTicketClaimed(ticketId);
    } catch (error: unknown) {
      if (error instanceof ApiError && error.status === 409) {
        setClaimError("This ticket has already been claimed");
        setRefreshVersion((version) => version + 1);
      } else {
        setClaimError(
          error instanceof Error
            ? error.message
            : "The ticket could not be claimed.",
        );
      }
    } finally {
      setIsClaiming(false);
    }
  }

  function refreshQueue() {
    setClaimError("");
    setRefreshVersion((version) => version + 1);
  }

  function changeSort(nextSort: SortOrder) {
    if (nextSort !== sort) {
      setClaimError("");
      setSort(nextSort);
    }
  }

  return (
    <div>
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-sm font-bold uppercase tracking-[0.2em] text-ocean">
            Moderator
          </p>
          <h1 className="mt-3 text-3xl font-bold">Appeal queue</h1>
          <p className="mt-3 max-w-2xl leading-7 text-ink/65">
            Review unclaimed appeals and claim a ticket to begin.
          </p>
        </div>
        <button
          className="rounded-lg border border-ink/15 px-4 py-2 text-sm font-semibold hover:bg-white disabled:opacity-60"
          disabled={isClaiming}
          onClick={refreshQueue}
          type="button"
        >
          Refresh
        </button>
      </div>

      <div className="mt-6 flex flex-wrap items-center gap-2">
        <span className="mr-1 text-sm font-semibold text-ink/65">Sort:</span>
        <button
          aria-pressed={sort === "asc"}
          className={`rounded-lg px-4 py-2 text-sm font-semibold ${
            sort === "asc" ? "bg-ocean text-white" : "border border-ink/15 hover:bg-white"
          }`}
          onClick={() => changeSort("asc")}
          type="button"
        >
          Oldest to latest
        </button>
        <button
          aria-pressed={sort === "desc"}
          className={`rounded-lg px-4 py-2 text-sm font-semibold ${
            sort === "desc" ? "bg-ocean text-white" : "border border-ink/15 hover:bg-white"
          }`}
          onClick={() => changeSort("desc")}
          type="button"
        >
          Latest to oldest
        </button>
      </div>

      {(claimError || listError) && (
        <p
          className="mt-6 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800"
          role="alert"
        >
          {claimError || listError}
        </p>
      )}

      {isLoading && (
        <p className="mt-6 text-sm text-ink/65" role="status">
          Loading appeals…
        </p>
      )}

      {tickets.length > 0 && (
        <div className="mt-6 space-y-3">
          {tickets.map((ticket) => (
            <AppealRow
              disabled={isClaiming}
              key={ticket.id}
              onSelect={() => void handleClaim(ticket.id)}
              ticket={ticket}
            />
          ))}
        </div>
      )}

      {!isLoading && !listError && tickets.length === 0 && (
        <p className="mt-6 rounded-2xl border border-ink/10 bg-white p-6 text-ink/65 shadow-sm">
          No appeals are waiting.
        </p>
      )}
    </div>
  );
}

export default QueuePage;
