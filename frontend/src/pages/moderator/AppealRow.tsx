import { useEffect, useState } from "react";
import type { TicketSummary } from "../../services/appealsShared";

export interface AppealRowProps {
  ticket: TicketSummary;
  onSelect: () => void;
  disabled?: boolean;
  showLockExpiry?: boolean;
}

function LockExpiryCountdown({ expiresAt }: { expiresAt: string }) {
  const expiration = Date.parse(expiresAt);
  const [now, setNow] = useState(Date.now);

  useEffect(() => {
    if (!Number.isFinite(expiration) || expiration <= Date.now()) return;

    const intervalId = window.setInterval(() => {
      const currentTime = Date.now();
      setNow(currentTime);
      if (currentTime >= expiration) window.clearInterval(intervalId);
    }, 1000);

    return () => window.clearInterval(intervalId);
  }, [expiration]);

  if (!Number.isFinite(expiration)) return null;

  const secondsRemaining = Math.max(0, Math.ceil((expiration - now) / 1000));
  const isExpired = secondsRemaining === 0;
  const isUrgent = secondsRemaining <= 5 * 60;
  const countdown = `${Math.floor(secondsRemaining / 60)}:${String(
    secondsRemaining % 60,
  ).padStart(2, "0")}`;
  const tone = isExpired
    ? "border-rose-200 bg-rose-50 text-rose-700"
    : isUrgent
      ? "border-amber-200 bg-amber-50 text-amber-800"
      : "border-ocean/15 bg-ocean/5 text-ocean";

  return (
    <span
      aria-label={isExpired ? "Appeal lock expired" : `Appeal lock expires in ${countdown}`}
      className={`inline-flex shrink-0 items-center gap-2 rounded-xl border px-3 py-2 ${tone}`}
      role="timer"
    >
      <svg
        aria-hidden="true"
        className="h-4 w-4"
        fill="none"
        viewBox="0 0 24 24"
        stroke="currentColor"
        strokeWidth="1.8"
      >
        <circle cx="12" cy="12" r="9" />
        <path strokeLinecap="round" strokeLinejoin="round" d="M12 7v5l3 2" />
      </svg>
      <span className="text-[10px] font-bold uppercase tracking-[0.12em]">
        {isExpired ? "Expired" : "Expires in"}
      </span>
      {!isExpired && (
        <span className="font-mono text-sm font-bold tabular-nums tracking-wide">
          {countdown}
        </span>
      )}
    </span>
  );
}

function AppealRow({
  ticket,
  onSelect,
  disabled = false,
  showLockExpiry = false,
}: AppealRowProps) {
  return (
    <button
      className="w-full rounded-2xl border border-ink/10 bg-white p-5 text-left shadow-sm transition hover:border-ocean/30 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ocean focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
      disabled={disabled}
      onClick={onSelect}
      type="button"
    >
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <span className="block font-semibold">
            {ticket.courseId} · {ticket.year} · {ticket.professor}
          </span>
          <span className="mt-1 block text-sm text-ink/65">
            {ticket.referenceCode}
          </span>
        </div>
        {showLockExpiry && ticket.lockExpiresAt && (
          <LockExpiryCountdown expiresAt={ticket.lockExpiresAt} />
        )}
      </div>
    </button>
  );
}

export default AppealRow;
