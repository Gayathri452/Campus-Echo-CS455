export const reviewCategories = [
  { value: "teaching", label: "Teaching" },
  { value: "content", label: "Course content" },
  { value: "assessment", label: "Assessment" },
  { value: "workload", label: "Workload" },
  { value: "resources", label: "Resources" },
  { value: "other", label: "Other" },
] as const;

export interface FeedbackInput {
  courseId: string;
  courseTitle: string;
  category: string;
  rating: number;
  content: string;
}

export interface FeedbackResponse extends FeedbackInput {
  referenceCode: string;
  status: string;
  createdAt: string;
  publishedAt?: string;
}

export interface CourseRating {
  courseId: string;
  courseTitle: string;
  reviewCount: number;
  averageRating: number;
}

export interface AppealResponse {
  referenceCode: string;
  reason: string;
  status: string;
  createdAt: string;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

async function requestJSON(
  path: string,
  options: RequestInit,
  fallback: string,
): Promise<unknown> {
  const response = await fetch(path, {
    ...options,
    credentials: "same-origin",
  });
  const text = await response.text();
  let result: unknown = null;
  try {
    result = text ? JSON.parse(text) : null;
  } catch {
    throw new Error(`The review service returned an invalid response (${response.status}).`);
  }
  if (!response.ok) {
    const message = isRecord(result) && typeof result.error === "string"
      ? result.error
      : fallback;
    throw new Error(message);
  }
  return result;
}

function isFeedback(value: unknown): value is FeedbackResponse {
  return isRecord(value) &&
    typeof value.referenceCode === "string" &&
    typeof value.courseId === "string" &&
    typeof value.courseTitle === "string" &&
    typeof value.category === "string" &&
    typeof value.rating === "number" &&
    typeof value.content === "string" &&
    typeof value.status === "string" &&
    typeof value.createdAt === "string";
}

function isCourseRating(value: unknown): value is CourseRating {
  return isRecord(value) &&
    typeof value.courseId === "string" &&
    typeof value.courseTitle === "string" &&
    typeof value.reviewCount === "number" &&
    typeof value.averageRating === "number";
}

export async function submitFeedback(
  input: FeedbackInput,
  accessToken?: string,
): Promise<FeedbackResponse> {
  const result = await requestJSON(
    "/api/v1/feedback",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      },
      body: JSON.stringify(input),
    },
    "We could not send your review. Please try again.",
  );
  if (!isFeedback(result)) {
    throw new Error("The server returned an unexpected review response.");
  }
  return result;
}

export async function exploreFeedback(
  filters: { search?: string; category?: string; courseId?: string; rating?: string },
  accessToken?: string,
): Promise<FeedbackResponse[]> {
  const params = new URLSearchParams();
  Object.entries(filters).forEach(([key, value]) => {
    if (value) params.set(key, value);
  });
  const result = await requestJSON(
    `/api/v1/feedback/explore?${params.toString()}`,
    { method: "GET", headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined },
    "We could not load published reviews.",
  );
  if (!Array.isArray(result) || !result.every(isFeedback)) {
    throw new Error("The server returned an unexpected review list.");
  }
  return result;
}

export async function getCourseRatings(
  courseId: string,
  accessToken?: string,
): Promise<CourseRating[]> {
  const query = courseId ? `?courseId=${encodeURIComponent(courseId)}` : "";
  const result = await requestJSON(
    `/api/v1/feedback/ratings${query}`,
    { method: "GET", headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined },
    "We could not load course ratings.",
  );
  if (!Array.isArray(result)) throw new Error("The server returned an unexpected ratings response.");
  return result.filter(isCourseRating);
}

export async function getReviewStatus(referenceCode: string, accessToken?: string): Promise<FeedbackResponse> {
  const result = await requestJSON(
    `/api/v1/feedback/status/${encodeURIComponent(referenceCode)}`,
    { method: "GET", headers: accessToken ? { Authorization: `Bearer ${accessToken}` } : undefined },
    "We could not load that review status.",
  );
  if (!isFeedback(result)) throw new Error("The server returned an unexpected status response.");
  return result;
}

export async function createAppeal(
  referenceCode: string,
  reason: string,
  accessToken?: string,
): Promise<AppealResponse> {
  const result = await requestJSON(
    `/api/v1/feedback/status/${encodeURIComponent(referenceCode)}/appeal`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      },
      body: JSON.stringify({ reason }),
    },
    "We could not submit the appeal.",
  );
  if (!isRecord(result) || typeof result.referenceCode !== "string" || typeof result.reason !== "string" || typeof result.status !== "string" || typeof result.createdAt !== "string") {
    throw new Error("The server returned an unexpected appeal response.");
  }
  return {
    referenceCode: result.referenceCode,
    reason: result.reason,
    status: result.status,
    createdAt: result.createdAt,
  };
}
