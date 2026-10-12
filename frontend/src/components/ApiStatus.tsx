import type { ApiHealth } from "../hooks/useApiHealth";

interface ApiStatusProps {
  health: ApiHealth;
}

const statusStyles: Record<ApiHealth, string> = {
  checking: "bg-amber-400",
  online: "bg-emerald-500",
  offline: "bg-rose-500",
};

const statusLabels: Record<ApiHealth, string> = {
  checking: "Checking feedback service…",
  online: "Feedback service is online",
  offline: "Feedback service is offline",
};

function ApiStatus({ health }: ApiStatusProps) {
  return (
    <div className="flex items-center gap-2 text-sm text-ink/60">
      <span
        aria-label={`API ${health}`}
        className={`h-2.5 w-2.5 rounded-full ${statusStyles[health]}`}
      />
      {statusLabels[health]}
    </div>
  );
}

export default ApiStatus;
