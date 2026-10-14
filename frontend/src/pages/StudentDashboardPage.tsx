import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import type { AuthSession } from "../services/auth";
import AnnouncementsPanel from "./student/AnnouncementsPanel";
import { CourseIdInput, CourseTitleAutofill } from "./student/CourseSuggestions";
import ReferenceNotebookPage from "./student/ReferenceNotebookPage";
import SaveToNotebook from "./student/SaveToNotebook";
import {
  createAppeal,
  exploreFeedback,
  getReviewStatus,
  reviewCategories,
  submitFeedback,
  type FeedbackResponse,
} from "../services/feedback";

type StudentView = "home" | "submit" | "explore" | "status" | "codes";

interface StudentDashboardPageProps {
  session: AuthSession;
  onLogout: () => void;
}

const statusLabel: Record<string, string> = {
  submitted: "Submitted",
  flagged: "Flagged for review",
  rejected: "Rejected",
  published: "Published",
  deleted: "Removed",
};

function StudentDashboardPage({ session, onLogout }: StudentDashboardPageProps) {
  const [view, setView] = useState<StudentView>("home");

  function navigate(nextView: StudentView) {
    setView(nextView);
  }

  return (
    <main className="min-h-screen bg-mist text-ink">
      <header className="border-b border-ink/10 bg-white">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 px-6 py-5">
          <span className="text-xl font-bold tracking-tight text-ocean">
            CampusEcho
          </span>
          <nav className="ml-auto flex flex-wrap items-center justify-end gap-2" aria-label="Student navigation">
            <NavButton active={view === "home"} onClick={() => navigate("home")}>Overview</NavButton>
            <NavButton active={view === "submit"} onClick={() => navigate("submit")}>Submit review</NavButton>
            <NavButton active={view === "explore"} onClick={() => navigate("explore")}>Explore reviews</NavButton>
            <NavButton active={view === "codes"} onClick={() => navigate("codes")}>My codes</NavButton>
            <NavButton active={view === "status"} onClick={() => navigate("status")}>Track status</NavButton>
            <button className="rounded-lg border border-ink/15 px-4 py-2 text-sm font-semibold hover:bg-mist" onClick={onLogout} type="button">
              Sign out
            </button>
          </nav>
        </div>
      </header>

      <div className="mx-auto max-w-5xl px-6 py-10">
        {view === "home" && <AnnouncementsPanel accessToken={session.accessToken} ownerId={session.user.id} />}
        {view === "home" && <DashboardHome onNavigate={navigate} />}
        {view === "submit" && <SubmitReview accessToken={session.accessToken} onComplete={() => navigate("status")} />}
        {view === "explore" && <ExploreReviews accessToken={session.accessToken} />}
        {view === "codes" && <ReferenceNotebookPage accessToken={session.accessToken} onTrack={() => navigate("status")} ownerId={session.user.id} />}
        {view === "status" && <ReviewStatus accessToken={session.accessToken} />}
      </div>
    </main>
  );
}

function NavButton({ active, children, onClick }: { active: boolean; children: ReactNode; onClick: () => void }) {
  return <button className={`rounded-lg px-4 py-2 text-sm font-semibold ${active ? "bg-ocean text-white" : "hover:bg-mist"}`} onClick={onClick} type="button">{children}</button>;
}

function DashboardHome({ onNavigate }: { onNavigate: (view: StudentView) => void }) {
  return (
    <section>
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div>
          <p className="text-sm font-bold uppercase tracking-[0.2em] text-ocean">Student</p>
          <h1 className="mt-3 text-3xl font-bold">Your feedback matters</h1>
          <p className="mt-3 max-w-2xl leading-7 text-ink/65">
            Share anonymous course feedback, explore published reviews, and track your submission by reference code.
          </p>
        </div>
      </div>
      <div className="mt-7 grid gap-4 md:grid-cols-3">
        <ActionCard title="Submit anonymously" text="Add course context, a category, a rating, and useful detail." action="Write a review" onClick={() => onNavigate("submit")} />
        <ActionCard title="Learn from others" text="Browse reviews that have been published by the review workflow." action="Explore reviews" onClick={() => onNavigate("explore")} />
        <ActionCard title="Keep your reference" text="Use your private reference code to check the current status." action="Track a review" onClick={() => onNavigate("status")} />
      </div>
    </section>
  );
}

function ActionCard({ title, text, action, onClick }: { title: string; text: string; action: string; onClick: () => void }) {
  return <article className="flex min-h-48 flex-col rounded-xl border border-ink/10 bg-white p-6 shadow-sm"><h3 className="text-lg font-bold">{title}</h3><p className="mt-3 text-sm leading-6 text-ink/60">{text}</p><button className="mt-auto pt-6 text-left text-sm font-semibold text-ocean hover:text-[#125a57]" onClick={onClick} type="button">{action}</button></article>;
}

function SubmitReview({ accessToken, onComplete }: { accessToken?: string; onComplete: () => void }) {
  const [form, setForm] = useState({ courseId: "", courseTitle: "", category: "teaching", rating: 0, content: "" });
  const [result, setResult] = useState<FeedbackResponse | null>(null);
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const setField = (field: keyof typeof form, value: string | number) => setForm((current) => ({ ...current, [field]: value }));

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    if (!form.rating) { setError("Choose a rating before submitting."); return; }
    setIsSubmitting(true);
    try {
      setResult(await submitFeedback(form, accessToken));
      setForm({ courseId: "", courseTitle: "", category: "teaching", rating: 0, content: "" });
    } catch (submitError: unknown) {
      setError(submitError instanceof Error ? submitError.message : "We could not submit the review.");
    } finally { setIsSubmitting(false); }
  }

  if (result) {
    return (
      <section className="rounded-xl border border-ink/10 bg-white p-6 shadow-sm">
        <p className="text-sm font-bold uppercase tracking-[0.2em] text-ocean">Review received</p>
        <h1 className="mt-3 text-3xl font-bold">Keep this reference code</h1>
        <p className="mt-3 leading-7 text-ink/65">
          Your review is stored anonymously. Save this code to track its status later.
        </p>
        <div className="mt-6 rounded-lg bg-mist px-5 py-5 text-center">
          <p className="text-xs font-bold uppercase tracking-[0.16em] text-ink/50">Private reference</p>
          <p className="mt-2 break-all font-mono text-2xl font-bold tracking-[0.12em] text-ocean">
            {result.referenceCode}
          </p>
        </div>
        <SaveToNotebook accessToken={accessToken} courseId={result.courseId} courseTitle={result.courseTitle} referenceCode={result.referenceCode} status={result.status} />
        <div className="mt-6 flex flex-wrap gap-3">
          <button className="rounded-lg bg-ocean px-4 py-2 text-sm font-semibold text-white hover:bg-ocean/90" onClick={onComplete} type="button">Track this review</button>
          <button className="rounded-lg border border-ink/15 px-4 py-2 text-sm font-semibold hover:bg-mist" onClick={() => setResult(null)} type="button">Submit another review</button>
        </div>
      </section>
    );
  }

  return (
    <section>
      <PageHeading
        eyebrow="Student"
        title="Submit a course review"
        description="Your email and student identity are never stored with this review."
      />
      <form
        className="mt-6 space-y-6 rounded-xl border border-ink/10 bg-white p-6 shadow-sm"
        onSubmit={handleSubmit}
      >
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label="Course ID">
            <CourseIdInput accessToken={accessToken} onChange={(value) => setField("courseId", value)} onPick={(course) => setField("courseTitle", course.title)} placeholder="CS455" value={form.courseId} />
          </Field>
          <Field label="Course title">
            <input className="input" placeholder="Software Engineering" value={form.courseTitle} onChange={(event) => setField("courseTitle", event.target.value)} />
          </Field>
        </div>
        <CourseTitleAutofill accessToken={accessToken} courseId={form.courseId} courseTitle={form.courseTitle} onFill={(title) => setField("courseTitle", title)} />
        <Field label="Category">
          <select className="input" value={form.category} onChange={(event) => setField("category", event.target.value)}>
            {reviewCategories.map((category) => <option key={category.value} value={category.value}>{category.label}</option>)}
          </select>
        </Field>
        <fieldset>
          <legend className="mb-3 text-sm font-semibold text-ink/65">Overall rating</legend>
          <div className="flex flex-wrap gap-2">
            {[1, 2, 3, 4, 5].map((rating) => (
              <button
                className={`rounded-lg border px-4 py-2 text-sm font-semibold ${form.rating === rating ? "border-ocean bg-ocean text-white" : "border-ink/15 hover:bg-mist"}`}
                key={rating}
                onClick={() => setField("rating", rating)}
                type="button"
              >
                {rating} / 5
              </button>
            ))}
          </div>
        </fieldset>
        <Field label="Your feedback">
          <textarea className="input min-h-44 resize-y" maxLength={5000} placeholder="What should future students or instructors know?" value={form.content} onChange={(event) => setField("content", event.target.value)} />
        </Field>
        {error && <p className="rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{error}</p>}
        <button className="rounded-lg bg-ocean px-4 py-2 text-sm font-semibold text-white hover:bg-ocean/90 disabled:cursor-wait disabled:opacity-60" disabled={isSubmitting} type="submit">
          {isSubmitting ? "Submitting review..." : "Submit anonymous review"}
        </button>
      </form>
    </section>
  );
}

function ExploreReviews({ accessToken }: { accessToken?: string }) {
  const [filters, setFilters] = useState({ search: "", category: "", courseId: "", rating: "" });
  const [reviews, setReviews] = useState<FeedbackResponse[]>([]);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  async function loadReviews() { setIsLoading(true); setError(""); try { setReviews(await exploreFeedback(filters, accessToken)); } catch (loadError: unknown) { setError(loadError instanceof Error ? loadError.message : "Could not load reviews."); } finally { setIsLoading(false); } }
  useEffect(() => { void loadReviews(); }, []);
  return <section><PageHeading eyebrow="Published feedback" title="Explore reviews" description="Search feedback that has completed the publication workflow." /><form className="mt-6 grid gap-3 rounded-xl border border-ink/10 bg-white p-6 shadow-sm md:grid-cols-[1.5fr_1fr_1fr_0.7fr_auto]" onSubmit={(event) => { event.preventDefault(); void loadReviews(); }}><input className="input" placeholder="Search course or feedback" value={filters.search} onChange={(event) => setFilters({ ...filters, search: event.target.value })} /><CourseIdInput accessToken={accessToken} onChange={(value) => setFilters((current) => ({ ...current, courseId: value }))} placeholder="Course ID" value={filters.courseId} /><select className="input" value={filters.category} onChange={(event) => setFilters({ ...filters, category: event.target.value })}><option value="">All categories</option>{reviewCategories.map((category) => <option key={category.value} value={category.value}>{category.label}</option>)}</select><select className="input" value={filters.rating} onChange={(event) => setFilters({ ...filters, rating: event.target.value })}><option value="">Any rating</option>{[1, 2, 3, 4, 5].map((rating) => <option key={rating} value={rating}>{rating} / 5</option>)}</select><button className="rounded-lg bg-ocean px-4 py-2 text-sm font-semibold text-white hover:bg-ocean/90" type="submit">Search</button></form>{error && <p className="mt-5 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{error}</p>}{isLoading ? <LoadingState /> : reviews.length === 0 ? <EmptyState title="No published reviews yet" text="Try a different filter or check back after the review workflow publishes new feedback." /> : <div className="mt-6 grid gap-4 lg:grid-cols-2">{reviews.map((review) => <ReviewCard key={review.referenceCode} review={review} />)}</div>}</section>;
}

function ReviewCard({ review }: { review: FeedbackResponse }) {
  return <article className="rounded-xl border border-ink/10 bg-white p-6 shadow-sm"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-sm font-semibold text-ocean">{review.courseId}</p><h3 className="mt-1 text-lg font-bold">{review.courseTitle}</h3></div><span className="rounded-full bg-mist px-3 py-1 text-xs font-semibold text-ink/60">{review.rating} / 5</span></div><p className="mt-4 text-sm leading-7 text-ink/70">{review.content}</p><p className="mt-5 text-xs font-semibold text-ink/45">{review.category}</p></article>;
}

function ReviewStatus({ accessToken }: { accessToken?: string }) {
  const [reference, setReference] = useState("");
  const [review, setReview] = useState<FeedbackResponse | null>(null);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  async function handleLookup(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setReview(null);
    setIsLoading(true);
    try {
      setReview(await getReviewStatus(reference.trim(), accessToken));
    } catch (loadError: unknown) {
      setError(loadError instanceof Error ? loadError.message : "Could not find that review.");
    } finally {
      setIsLoading(false);
    }
  }
  return <section><PageHeading eyebrow="Private reference" title="Track review status" description="Enter the reference code you received after submitting your review." /><form className="mt-6 flex max-w-xl gap-3" onSubmit={handleLookup}><input className="input font-mono uppercase" placeholder="CE-XXXXXXXXXXXX" value={reference} onChange={(event) => setReference(event.target.value.toUpperCase())} /><button className="rounded-lg bg-ocean px-4 py-2 text-sm font-semibold text-white hover:bg-ocean/90" type="submit">Check status</button></form>{isLoading && <LoadingState />}{error && <p className="mt-5 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{error}</p>}{review && <StatusCard key={review.referenceCode} review={review} accessToken={accessToken} />}</section>;
}

function StatusCard({ review, accessToken }: { review: FeedbackResponse; accessToken?: string }) {
  const [reason, setReason] = useState("");
  const [appealMessage, setAppealMessage] = useState("");
  const [appealError, setAppealError] = useState("");
  const [isAppealing, setIsAppealing] = useState(false);
  const [appealStatus, setAppealStatus] = useState(review.appealStatus ?? "");
  const eligible = (review.status === "flagged" || review.status === "rejected") && !appealStatus;

  async function submitAppeal(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setAppealError("");
    setAppealMessage("");
    setIsAppealing(true);
    try {
      const appeal = await createAppeal(review.referenceCode, reason, accessToken);
      setAppealStatus(appeal.status);
      setAppealMessage("Your appeal has been sent to the moderator queue.");
      setReason("");
    } catch (submitError: unknown) {
      setAppealError(submitError instanceof Error ? submitError.message : "Could not submit appeal.");
    } finally {
      setIsAppealing(false);
    }
  }

  const appealStatusLabel: Record<string, string> = {
    under_appeal: "Waiting for a moderator",
    locked: "A moderator is reviewing this appeal",
    restored: "The review was published after appeal",
    deleted: "The review was removed after appeal",
  };

  return <article className="mt-6 rounded-xl border border-ink/10 bg-white p-6 shadow-sm"><div className="flex flex-wrap items-center justify-between gap-4"><div><p className="text-sm font-semibold text-ocean">{review.courseId}</p><h2 className="mt-1 text-xl font-bold">{review.courseTitle}</h2></div><span className="rounded-full bg-mist px-3 py-1 text-sm font-semibold">{statusLabel[review.status] ?? review.status}</span></div><p className="mt-5 text-sm leading-7 text-ink/65">{review.content}</p>{review.flagReason && <p className="mt-4 text-sm text-ink/60"><span className="font-bold">Flag reason:</span> {review.flagReason}</p>}{appealStatus && <div className="mt-6 border-t border-ink/10 pt-5"><p className="text-sm font-bold">{appealStatusLabel[appealStatus] ?? "Appeal status"}{!appealStatusLabel[appealStatus] && <span className="font-medium"> · {appealStatus}</span>}</p>{review.appealResolutionComment && <p className="mt-2 text-sm leading-6 text-ink/60">Moderator note: {review.appealResolutionComment}</p>}</div>}{eligible ? <form className="mt-7 border-t border-ink/10 pt-6" onSubmit={submitAppeal}><h3 className="font-bold">Request an appeal</h3><p className="mt-2 text-sm leading-6 text-ink/60">Explain why this review should be reconsidered. You may appeal only once.</p><textarea className="input mt-4 min-h-28" maxLength={2000} placeholder="Your appeal reason" value={reason} onChange={(event) => setReason(event.target.value)} />{appealError && <p className="mt-3 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{appealError}</p>}{appealMessage && <p className="mt-3 rounded-lg bg-emerald-50 px-4 py-3 text-sm text-emerald-800" role="status">{appealMessage}</p>}<button className="mt-4 rounded-lg bg-ocean px-4 py-2 text-sm font-semibold text-white hover:bg-ocean/90 disabled:opacity-60" disabled={isAppealing || !reason.trim()} type="submit">{isAppealing ? "Submitting appeal..." : "Submit appeal"}</button></form> : !appealStatus && <p className="mt-6 border-t border-ink/10 pt-5 text-sm text-ink/55">This review is not currently eligible for an appeal.</p>}</article>;
}

function PageHeading({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) {
  return <div><p className="text-sm font-bold uppercase tracking-[0.2em] text-ocean">{eyebrow}</p><h1 className="mt-3 text-3xl font-bold">{title}</h1><p className="mt-3 max-w-2xl leading-7 text-ink/65">{description}</p></div>;
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block"><span className="mb-2 block text-sm font-semibold text-ink/65">{label}</span>{children}</label>;
}

function LoadingState() {
  return <p className="mt-6 text-sm text-ink/65" role="status">Loading published data...</p>;
}

function EmptyState({ title, text }: { title: string; text: string }) {
  return <div className="mt-6 rounded-xl border border-ink/10 bg-white p-6 text-ink/65 shadow-sm"><h2 className="font-bold text-ink">{title}</h2><p className="mt-2 max-w-2xl text-sm leading-6">{text}</p></div>;
}

export default StudentDashboardPage;
