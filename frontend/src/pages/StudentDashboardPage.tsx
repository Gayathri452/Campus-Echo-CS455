import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import ApiStatus from "../components/ApiStatus";
import { useApiHealth } from "../hooks/useApiHealth";
import type { AuthSession } from "../services/auth";
import {
  createAppeal,
  exploreFeedback,
  getCourseRatings,
  getReviewStatus,
  reviewCategories,
  submitFeedback,
  type CourseRating,
  type FeedbackResponse,
} from "../services/feedback";

type StudentView = "home" | "submit" | "explore" | "ratings" | "status";

interface StudentDashboardPageProps {
  session: AuthSession;
  onLogout: () => void;
}

const statusLabel: Record<string, string> = {
  submitted: "Submitted",
  flagged: "Needs attention",
  rejected: "Rejected",
  published: "Published",
};

function StudentDashboardPage({ session, onLogout }: StudentDashboardPageProps) {
  const [view, setView] = useState<StudentView>("home");
  const apiHealth = useApiHealth();

  function navigate(nextView: StudentView) {
    setView(nextView);
  }

  return (
    <main className="min-h-screen bg-mist text-ink">
      <header className="border-b border-ink/10 bg-white">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 px-5 py-4 sm:px-8">
          <button className="text-left text-xl font-extrabold tracking-tight text-ocean" onClick={() => navigate("home")} type="button">
            CampusEcho
          </button>
          <nav className="order-3 flex w-full gap-1 overflow-x-auto pb-1 sm:order-none sm:w-auto sm:pb-0" aria-label="Student navigation">
            <NavButton active={view === "home"} onClick={() => navigate("home")}>Overview</NavButton>
            <NavButton active={view === "submit"} onClick={() => navigate("submit")}>Submit review</NavButton>
            <NavButton active={view === "explore"} onClick={() => navigate("explore")}>Explore reviews</NavButton>
            <NavButton active={view === "ratings"} onClick={() => navigate("ratings")}>Course ratings</NavButton>
            <NavButton active={view === "status"} onClick={() => navigate("status")}>Track status</NavButton>
          </nav>
          <div className="flex items-center gap-3">
            <span className="hidden max-w-48 truncate text-sm text-ink/55 md:inline">{session.user.email}</span>
            <button className="rounded-lg border border-ink/15 px-3 py-2 text-sm font-bold hover:bg-mist" onClick={onLogout} type="button">Sign out</button>
          </div>
        </div>
      </header>

      <div className="mx-auto max-w-7xl px-5 py-8 sm:px-8 sm:py-12">
        {view === "home" && <DashboardHome onNavigate={navigate} health={apiHealth} />}
        {view === "submit" && <SubmitReview accessToken={session.accessToken} onComplete={() => navigate("status")} />}
        {view === "explore" && <ExploreReviews accessToken={session.accessToken} />}
        {view === "ratings" && <CourseRatingsView accessToken={session.accessToken} />}
        {view === "status" && <ReviewStatus accessToken={session.accessToken} />}
      </div>
    </main>
  );
}

function NavButton({ active, children, onClick }: { active: boolean; children: ReactNode; onClick: () => void }) {
  return <button className={`whitespace-nowrap rounded-lg px-3 py-2 text-sm font-bold transition ${active ? "bg-ocean text-white" : "text-ink/60 hover:bg-mist hover:text-ink"}`} onClick={onClick} type="button">{children}</button>;
}

function DashboardHome({ onNavigate, health }: { onNavigate: (view: StudentView) => void; health: ReturnType<typeof useApiHealth> }) {
  return (
    <section className="space-y-8">
      <div className="grid gap-8 overflow-hidden rounded-3xl bg-[#173b38] px-7 py-9 text-white shadow-sm sm:px-10 sm:py-12 lg:grid-cols-[1.2fr_0.8fr] lg:items-end">
        <div>
          <p className="text-xs font-extrabold uppercase tracking-[0.2em] text-[#78c7a3]">Student workspace</p>
          <h1 className="mt-4 max-w-2xl font-[DM_Sans] text-4xl font-semibold tracking-tight sm:text-6xl">Your perspective can shape the next class.</h1>
          <p className="mt-5 max-w-xl text-base leading-7 text-white/70">Share anonymous, constructive course feedback and use the reference code to follow its progress.</p>
        </div>
        <div className="rounded-2xl border border-white/15 bg-white/10 p-5">
          <p className="text-sm font-bold text-white/70">Ready to contribute?</p>
          <p className="mt-2 text-2xl font-semibold">Start a new review</p>
          <button className="mt-5 rounded-lg bg-[#d6e6ad] px-4 py-3 text-sm font-extrabold text-[#173b38] hover:bg-white" onClick={() => onNavigate("submit")} type="button">Submit review</button>
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div>
          <p className="text-xs font-extrabold uppercase tracking-[0.18em] text-ocean">What you can do</p>
          <h2 className="mt-2 text-2xl font-extrabold tracking-tight">A quieter way to participate</h2>
        </div>
        <ApiStatus health={health} />
      </div>
      <div className="grid gap-4 md:grid-cols-3">
        <ActionCard title="Submit anonymously" text="Add course context, a category, a rating, and useful detail." action="Write a review" onClick={() => onNavigate("submit")} />
        <ActionCard title="Learn from others" text="Browse reviews that have been published by the review workflow." action="Explore reviews" onClick={() => onNavigate("explore")} />
        <ActionCard title="Keep your reference" text="Use your private reference code to check the current status." action="Track a review" onClick={() => onNavigate("status")} />
      </div>
    </section>
  );
}

function ActionCard({ title, text, action, onClick }: { title: string; text: string; action: string; onClick: () => void }) {
  return <article className="flex min-h-48 flex-col rounded-2xl border border-ink/10 bg-white p-6 shadow-sm"><h3 className="text-lg font-extrabold">{title}</h3><p className="mt-3 text-sm leading-6 text-ink/60">{text}</p><button className="mt-auto pt-6 text-left text-sm font-extrabold text-ocean hover:text-[#125a57]" onClick={onClick} type="button">{action}</button></article>;
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
    return <section className="mx-auto max-w-2xl rounded-3xl border border-emerald-200 bg-white p-7 shadow-sm sm:p-10"><p className="text-xs font-extrabold uppercase tracking-[0.18em] text-emerald-700">Review received</p><h1 className="mt-3 text-3xl font-extrabold tracking-tight">Keep this reference code</h1><p className="mt-3 leading-7 text-ink/65">Your review is stored anonymously. Save this code to track its status later.</p><div className="mt-7 rounded-2xl bg-mist px-5 py-5 text-center"><p className="text-xs font-extrabold uppercase tracking-[0.18em] text-ink/50">Private reference</p><p className="mt-2 font-mono text-2xl font-bold tracking-[0.16em] text-ocean">{result.referenceCode}</p></div><div className="mt-7 flex flex-wrap gap-3"><button className="rounded-lg bg-ocean px-4 py-3 text-sm font-extrabold text-white hover:bg-[#125a57]" onClick={onComplete} type="button">Track this review</button><button className="rounded-lg border border-ink/15 px-4 py-3 text-sm font-extrabold hover:bg-mist" onClick={() => setResult(null)} type="button">Submit another review</button></div></section>;
  }

  return <section className="mx-auto max-w-3xl"><PageHeading eyebrow="Anonymous submission" title="Submit a course review" description="Your email and student identity are never stored with this review." /><form className="mt-8 space-y-6 rounded-3xl border border-ink/10 bg-white p-6 shadow-sm sm:p-9" onSubmit={handleSubmit}><div className="grid gap-5 sm:grid-cols-2"><Field label="Course ID"><input className="input" placeholder="CS455" value={form.courseId} onChange={(event) => setField("courseId", event.target.value)} /></Field><Field label="Course title"><input className="input" placeholder="Software Engineering" value={form.courseTitle} onChange={(event) => setField("courseTitle", event.target.value)} /></Field></div><Field label="Category"><select className="input" value={form.category} onChange={(event) => setField("category", event.target.value)}>{reviewCategories.map((category) => <option key={category.value} value={category.value}>{category.label}</option>)}</select></Field><fieldset><legend className="mb-3 text-sm font-extrabold text-ink/70">Overall rating</legend><div className="flex flex-wrap gap-2">{[1, 2, 3, 4, 5].map((rating) => <button className={`rounded-lg border px-4 py-3 text-sm font-extrabold ${form.rating === rating ? "border-ocean bg-ocean text-white" : "border-ink/15 hover:bg-mist"}`} key={rating} onClick={() => setField("rating", rating)} type="button">{rating} / 5</button>)}</div></fieldset><Field label="Your feedback"><textarea className="input min-h-44 resize-y" maxLength={5000} placeholder="What should future students or instructors know?" value={form.content} onChange={(event) => setField("content", event.target.value)} /></Field>{error && <p className="rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{error}</p>}<button className="rounded-lg bg-ocean px-5 py-3 text-sm font-extrabold text-white hover:bg-[#125a57] disabled:cursor-wait disabled:opacity-60" disabled={isSubmitting} type="submit">{isSubmitting ? "Submitting review..." : "Submit anonymous review"}</button></form></section>;
}

function ExploreReviews({ accessToken }: { accessToken?: string }) {
  const [filters, setFilters] = useState({ search: "", category: "", courseId: "", rating: "" });
  const [reviews, setReviews] = useState<FeedbackResponse[]>([]);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  async function loadReviews() { setIsLoading(true); setError(""); try { setReviews(await exploreFeedback(filters, accessToken)); } catch (loadError: unknown) { setError(loadError instanceof Error ? loadError.message : "Could not load reviews."); } finally { setIsLoading(false); } }
  useEffect(() => { void loadReviews(); }, []);
  return <section><PageHeading eyebrow="Published feedback" title="Explore reviews" description="Search feedback that has completed the publication workflow." /><form className="mt-8 grid gap-3 rounded-2xl border border-ink/10 bg-white p-5 shadow-sm md:grid-cols-[1.5fr_1fr_1fr_0.7fr_auto]" onSubmit={(event) => { event.preventDefault(); void loadReviews(); }}><input className="input" placeholder="Search course or feedback" value={filters.search} onChange={(event) => setFilters({ ...filters, search: event.target.value })} /><input className="input" placeholder="Course ID" value={filters.courseId} onChange={(event) => setFilters({ ...filters, courseId: event.target.value })} /><select className="input" value={filters.category} onChange={(event) => setFilters({ ...filters, category: event.target.value })}><option value="">All categories</option>{reviewCategories.map((category) => <option key={category.value} value={category.value}>{category.label}</option>)}</select><select className="input" value={filters.rating} onChange={(event) => setFilters({ ...filters, rating: event.target.value })}><option value="">Any rating</option>{[1, 2, 3, 4, 5].map((rating) => <option key={rating} value={rating}>{rating} / 5</option>)}</select><button className="rounded-lg bg-ocean px-4 py-3 text-sm font-extrabold text-white" type="submit">Search</button></form>{error && <p className="mt-5 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{error}</p>}{isLoading ? <LoadingState /> : reviews.length === 0 ? <EmptyState title="No published reviews yet" text="Try a different filter or check back after the review workflow publishes new feedback." /> : <div className="mt-6 grid gap-4 lg:grid-cols-2">{reviews.map((review) => <ReviewCard key={review.referenceCode} review={review} />)}</div>}</section>;
}

function ReviewCard({ review }: { review: FeedbackResponse }) {
  return <article className="rounded-2xl border border-ink/10 bg-white p-6 shadow-sm"><div className="flex flex-wrap items-start justify-between gap-3"><div><p className="text-xs font-extrabold uppercase tracking-[0.14em] text-ocean">{review.courseId}</p><h3 className="mt-1 text-lg font-extrabold">{review.courseTitle}</h3></div><span className="rounded-full bg-mist px-3 py-1 text-xs font-bold text-ink/60">{review.rating} / 5</span></div><p className="mt-4 text-sm leading-7 text-ink/70">{review.content}</p><p className="mt-5 text-xs font-bold text-ink/45">{review.category}</p></article>;
}

function CourseRatingsView({ accessToken }: { accessToken?: string }) {
  const [courseId, setCourseId] = useState("");
  const [ratings, setRatings] = useState<CourseRating[]>([]);
  const [reviews, setReviews] = useState<FeedbackResponse[]>([]);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  async function loadRatings() { setIsLoading(true); setError(""); try { const [nextRatings, nextReviews] = await Promise.all([getCourseRatings(courseId, accessToken), courseId.trim() ? exploreFeedback({ courseId: courseId.trim() }, accessToken) : Promise.resolve([])]); setRatings(nextRatings); setReviews(nextReviews); } catch (loadError: unknown) { setError(loadError instanceof Error ? loadError.message : "Could not load course ratings."); } finally { setIsLoading(false); } }
  useEffect(() => { void loadRatings(); }, []);
  return <section><PageHeading eyebrow="Published signals" title="Course ratings" description="Compare average ratings from published anonymous reviews." /><form className="mt-8 flex max-w-xl gap-3" onSubmit={(event) => { event.preventDefault(); void loadRatings(); }}><input className="input" placeholder="Filter by course ID" value={courseId} onChange={(event) => setCourseId(event.target.value)} /><button className="rounded-lg bg-ocean px-4 py-3 text-sm font-extrabold text-white" type="submit">Filter</button></form>{error && <p className="mt-5 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{error}</p>}{isLoading ? <LoadingState /> : ratings.length === 0 ? <EmptyState title="No published ratings yet" text="Ratings will appear here when reviews complete the publication workflow." /> : <><div className="mt-7 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{ratings.map((rating) => <article className="rounded-2xl border border-ink/10 bg-white p-6 shadow-sm" key={rating.courseId}><p className="text-xs font-extrabold uppercase tracking-[0.14em] text-ocean">{rating.courseId}</p><h3 className="mt-2 font-extrabold">{rating.courseTitle}</h3><p className="mt-6 text-4xl font-extrabold text-ink">{rating.averageRating.toFixed(1)}<span className="ml-1 text-base text-ink/45">/ 5</span></p><p className="mt-2 text-sm text-ink/55">Based on {rating.reviewCount} published review{rating.reviewCount === 1 ? "" : "s"}</p></article>)}</div>{courseId.trim() && <div className="mt-10"><h2 className="text-2xl font-extrabold">Published feedback for {courseId.trim()}</h2>{reviews.length === 0 ? <EmptyState title="No matching published reviews" text="The course has a rating summary but no review text matched this filter." /> : <div className="mt-5 grid gap-4 lg:grid-cols-2">{reviews.map((review) => <ReviewCard key={review.referenceCode} review={review} />)}</div>}</div>}</> }</section>;
}

function ReviewStatus({ accessToken }: { accessToken?: string }) {
  const [reference, setReference] = useState("");
  const [review, setReview] = useState<FeedbackResponse | null>(null);
  const [error, setError] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  async function handleLookup(event: FormEvent<HTMLFormElement>) { event.preventDefault(); setError(""); setReview(null); setIsLoading(true); try { setReview(await getReviewStatus(reference.trim(), accessToken)); } catch (loadError: unknown) { setError(loadError instanceof Error ? loadError.message : "Could not find that review."); } finally { setIsLoading(false); } }
  return <section className="mx-auto max-w-3xl"><PageHeading eyebrow="Private reference" title="Track review status" description="Enter the reference code you received after submitting your review." /><form className="mt-8 flex max-w-xl gap-3" onSubmit={handleLookup}><input className="input font-mono uppercase" placeholder="CE-XXXXXXXXXXXX" value={reference} onChange={(event) => setReference(event.target.value.toUpperCase())} /><button className="rounded-lg bg-ocean px-4 py-3 text-sm font-extrabold text-white" type="submit">Check status</button></form>{isLoading && <LoadingState />}{error && <p className="mt-5 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{error}</p>}{review && <StatusCard review={review} accessToken={accessToken} />}</section>;
}

function StatusCard({ review, accessToken }: { review: FeedbackResponse; accessToken?: string }) {
  const [reason, setReason] = useState("");
  const [appealMessage, setAppealMessage] = useState("");
  const [appealError, setAppealError] = useState("");
  const [isAppealing, setIsAppealing] = useState(false);
  const eligible = review.status === "flagged" || review.status === "rejected";
  async function submitAppeal(event: FormEvent<HTMLFormElement>) { event.preventDefault(); setAppealError(""); setAppealMessage(""); setIsAppealing(true); try { const appeal = await createAppeal(review.referenceCode, reason, accessToken); setAppealMessage(`Appeal ${appeal.status}.`); setReason(""); } catch (submitError: unknown) { setAppealError(submitError instanceof Error ? submitError.message : "Could not submit appeal."); } finally { setIsAppealing(false); } }
  return <article className="mt-8 rounded-3xl border border-ink/10 bg-white p-7 shadow-sm"><div className="flex flex-wrap items-center justify-between gap-4"><div><p className="text-xs font-extrabold uppercase tracking-[0.16em] text-ocean">{review.courseId}</p><h2 className="mt-1 text-xl font-extrabold">{review.courseTitle}</h2></div><span className="rounded-full bg-mist px-3 py-1 text-sm font-extrabold">{statusLabel[review.status] ?? review.status}</span></div><p className="mt-5 text-sm leading-7 text-ink/65">{review.content}</p>{eligible ? <form className="mt-7 border-t border-ink/10 pt-6" onSubmit={submitAppeal}><h3 className="font-extrabold">Request an appeal</h3><p className="mt-2 text-sm leading-6 text-ink/60">Explain why this review should be reconsidered.</p><textarea className="input mt-4 min-h-28" maxLength={2000} placeholder="Your appeal reason" value={reason} onChange={(event) => setReason(event.target.value)} />{appealError && <p className="mt-3 rounded-lg bg-rose-50 px-4 py-3 text-sm text-rose-800" role="alert">{appealError}</p>}{appealMessage && <p className="mt-3 rounded-lg bg-emerald-50 px-4 py-3 text-sm text-emerald-800" role="status">{appealMessage}</p>}<button className="mt-4 rounded-lg bg-ocean px-4 py-3 text-sm font-extrabold text-white disabled:opacity-60" disabled={isAppealing || !reason.trim()} type="submit">{isAppealing ? "Submitting appeal..." : "Submit appeal"}</button></form> : <p className="mt-6 border-t border-ink/10 pt-5 text-sm text-ink/55">Appeals become available only if the moderation workflow flags or rejects this review.</p>}</article>;
}

function PageHeading({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) {
  return <div><p className="text-xs font-extrabold uppercase tracking-[0.18em] text-ocean">{eyebrow}</p><h1 className="mt-3 text-4xl font-extrabold tracking-tight sm:text-5xl">{title}</h1><p className="mt-4 max-w-2xl text-base leading-7 text-ink/60">{description}</p></div>;
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <label className="block"><span className="mb-2 block text-sm font-extrabold text-ink/70">{label}</span>{children}</label>;
}

function LoadingState() {
  return <div className="mt-7 rounded-2xl border border-ink/10 bg-white p-8 text-sm font-semibold text-ink/55">Loading published data...</div>;
}

function EmptyState({ title, text }: { title: string; text: string }) {
  return <div className="mt-7 rounded-2xl border border-dashed border-ink/20 bg-white p-9 text-center"><h2 className="font-extrabold">{title}</h2><p className="mx-auto mt-2 max-w-md text-sm leading-6 text-ink/55">{text}</p></div>;
}

export default StudentDashboardPage;
