import { useState, type FormEvent, type KeyboardEvent } from "react";

interface DeleteModalProps {
  isSubmitting: boolean;
  onCancel: () => void;
  onSubmit: (comment: string) => void;
}

function DeleteModal({ isSubmitting, onCancel, onSubmit }: DeleteModalProps) {
  const [comment, setComment] = useState("");

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const trimmedComment = comment.trim();
    if (trimmedComment) onSubmit(trimmedComment);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape" && !isSubmitting) onCancel();
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink/50 p-4"
      onKeyDown={handleKeyDown}
    >
      <section
        aria-labelledby="delete-appeal-title"
        aria-modal="true"
        className="w-full max-w-lg rounded-xl bg-white p-6 shadow-xl"
        role="dialog"
      >
        <h2 className="text-xl font-bold" id="delete-appeal-title">
          Delete review
        </h2>
        <p className="mt-2 text-sm leading-6 text-ink/65">
          Add a comment explaining why this review breaks the guidelines.
        </p>

        <form className="mt-5" onSubmit={handleSubmit}>
          <label className="block">
            <span className="mb-2 block text-sm font-semibold">Resolution comment</span>
            <textarea
              autoFocus
              className="min-h-32 w-full resize-y rounded-lg border border-ink/15 px-3 py-2 outline-none focus:border-ocean focus:ring-2 focus:ring-ocean/15"
              onChange={(event) => setComment(event.target.value)}
              required
              value={comment}
            />
          </label>

          <div className="mt-6 flex justify-end gap-3">
            <button
              className="rounded-lg border border-ink/15 px-4 py-2 text-sm font-semibold hover:bg-mist disabled:opacity-60"
              disabled={isSubmitting}
              onClick={onCancel}
              type="button"
            >
              Cancel
            </button>
            <button
              className="rounded-lg bg-rose-700 px-4 py-2 text-sm font-semibold text-white hover:bg-rose-800 disabled:cursor-not-allowed disabled:opacity-60"
              disabled={isSubmitting || comment.trim().length === 0}
              type="submit"
            >
              {isSubmitting ? "Submitting..." : "Submit"}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}

export default DeleteModal;