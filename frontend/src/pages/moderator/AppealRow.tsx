import type { TicketSummary } from "../../services/appealsShared";

export interface AppealRowProps {
  ticket: TicketSummary;
  onSelect: () => void;
  disabled?: boolean;
}

function AppealRow({ ticket, onSelect, disabled = false }: AppealRowProps) {
  return (
    <button
      className="w-full rounded-2xl border border-ink/10 bg-white p-5 text-left shadow-sm transition hover:border-ocean/30 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ocean focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-60"
      disabled={disabled}
      onClick={onSelect}
      type="button"
    >
      <span className="block font-semibold">
        {ticket.courseId} · {ticket.year} · {ticket.professor}
      </span>
      <span className="mt-1 block text-sm text-ink/65">
        {ticket.referenceCode}
      </span>
    </button>
  );
}

export default AppealRow;
