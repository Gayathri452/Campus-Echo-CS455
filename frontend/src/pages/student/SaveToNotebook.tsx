import { useEffect, useState } from "react";
import { addToNotebook, getNotebookOwner } from "../../services/referenceNotebook";

// Shown under the reference code right after a review is submitted. It saves
// the code to "My codes" automatically, because the code is only displayed
// once.

interface SaveToNotebookProps {
  accessToken?: string;
  referenceCode: string;
  courseId: string;
  courseTitle: string;
  status?: string;
}

type SaveState = "saving" | "saved" | "failed";

function SaveToNotebook({ accessToken, referenceCode, courseId, courseTitle, status }: SaveToNotebookProps) {
  const [state, setState] = useState<SaveState>("saving");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    let cancelled = false;
    if (!accessToken) {
      setState("failed");
      return;
    }
    getNotebookOwner(accessToken)
      .then((ownerId) => {
        const saved = addToNotebook(ownerId, { referenceCode, courseId, courseTitle, lastStatus: status });
        if (!cancelled) setState(saved ? "saved" : "failed");
      })
      .catch(() => {
        if (!cancelled) setState("failed");
      });
    return () => {
      cancelled = true;
    };
  }, [accessToken, referenceCode, courseId, courseTitle, status]);

  async function copyCode() {
    try {
      await navigator.clipboard.writeText(referenceCode);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-3 rounded-lg border border-ink/10 px-4 py-3 text-sm">
      <p className={state === "failed" ? "text-amber-900" : "text-ink/70"} role="status">
        {state === "saving" && "Saving to My codes…"}
        {state === "saved" && "✓ Saved to My codes on this device. You can add a note to it there."}
        {state === "failed" && "This browser could not save the code. Copy it and keep it somewhere safe."}
      </p>
      <button
        className="rounded-lg border border-ink/15 px-3 py-1.5 font-semibold hover:bg-mist"
        onClick={() => void copyCode()}
        type="button"
      >
        {copied ? "Copied" : "Copy code"}
      </button>
    </div>
  );
}

export default SaveToNotebook;
