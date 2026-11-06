import { describe, expect, it, vi } from "vitest";
import { headersOf, mockFetch } from "../test/fetchMock";
import {
  createAppeal,
  exploreFeedback,
  getCourseRatings,
  getReviewStatus,
  reviewCategories,
  submitFeedback,
  type FeedbackInput,
} from "./feedback";

const review = {
  referenceCode: "CE-ABC123",
  courseId: "CS455",
  courseTitle: "Software Engineering",
  professor: "Prof. Rao",
  year: 2026,
  category: "teaching",
  rating: 4,
  content: "Clear and useful lectures.",
  status: "published",
  createdAt: "2026-11-01T00:00:00Z",
};
const input: FeedbackInput = {
  courseId: "CS455",
  courseTitle: "Software Engineering",
  professor: "Prof. Rao",
  year: 2026,
  category: "teaching",
  rating: 4,
  content: "Clear and useful lectures.",
};

describe("feedback service", () => {
  it("submits feedback with JSON and optional bearer authorization", async () => {
    const fetchMock = mockFetch({ "/api/v1/feedback": { status: 201, body: review } });

    await expect(submitFeedback(input, "student-token")).resolves.toEqual(review);
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    });
    expect(headersOf(fetchMock)).toHaveProperty("Authorization");

    const cookieFetch = mockFetch({ "/api/v1/feedback": { status: 201, body: review } });
    await submitFeedback(input);
    expect(headersOf(cookieFetch)).not.toHaveProperty("Authorization");
  });

  it("rejects malformed feedback and reports backend and transport failures", async () => {
    mockFetch({ "/api/v1/feedback": { body: { ...review, rating: "four" } } });
    await expect(submitFeedback(input)).rejects.toThrow("unexpected review response");

    mockFetch({ "/api/v1/feedback": { status: 400, body: { error: "course is unavailable" } } });
    await expect(submitFeedback(input)).rejects.toThrow("course is unavailable");

    mockFetch({ "/api/v1/feedback": { status: 503, body: {} } });
    await expect(submitFeedback(input)).rejects.toThrow("We could not send your review.");

    mockFetch({ "/api/v1/feedback": { status: 502, raw: "bad gateway" } });
    await expect(submitFeedback(input)).rejects.toThrow("invalid response (502)");

    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new TypeError("network disconnected")));
    await expect(submitFeedback(input)).rejects.toThrow(
      "Could not reach the review service. Check your connection and try again.",
    );
  });

  it("builds explore filters, omits empty filters, and validates the returned list", async () => {
    const fetchMock = mockFetch({ "/api/v1/feedback/explore": { body: [review] } });

    await expect(exploreFeedback({
      search: "  grading policy ",
      category: "teaching",
      courseId: "",
      rating: "5",
    }, "student-token")).resolves.toEqual([review]);
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/feedback/explore?search=++grading+policy+&category=teaching&rating=5",
    );
    expect(headersOf(fetchMock)).toHaveProperty("Authorization");

    mockFetch({ "/api/v1/feedback/explore": { body: { reviews: [review] } } });
    await expect(exploreFeedback({})).rejects.toThrow("unexpected review list");
    mockFetch({ "/api/v1/feedback/explore": { body: [review, { ...review, year: "2026" }] } });
    await expect(exploreFeedback({})).rejects.toThrow("unexpected review list");
  });

  it("loads and validates course ratings while ignoring malformed rows", async () => {
    const ratings = [
      { courseId: "CS455", courseTitle: "Software Engineering", reviewCount: 8, averageRating: 4.25 },
      { courseId: "BAD", courseTitle: "Malformed", reviewCount: "many", averageRating: 3 },
    ];
    const fetchMock = mockFetch({ "/api/v1/feedback/ratings": { body: ratings } });

    await expect(getCourseRatings("")).resolves.toEqual([ratings[0]]);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/feedback/ratings");
    mockFetch({ "/api/v1/feedback/ratings": { body: {} } });
    await expect(getCourseRatings("CS 455")).rejects.toThrow("unexpected ratings response");

    const encodedFetch = mockFetch({ "/api/v1/feedback/ratings": { body: [] } });
    await getCourseRatings("CS/455");
    expect(encodedFetch.mock.calls[0][0]).toBe("/api/v1/feedback/ratings?courseId=CS%2F455");
  });

  it("encodes review references and validates status data", async () => {
    const fetchMock = mockFetch({ "/api/v1/feedback/status/CE%2FABC": { body: review } });

    await expect(getReviewStatus("CE/ABC")).resolves.toEqual(review);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/feedback/status/CE%2FABC");
    mockFetch({ "/api/v1/feedback/status/bad": { body: { referenceCode: "bad" } } });
    await expect(getReviewStatus("bad")).rejects.toThrow("unexpected status response");
  });

  it("creates appeals and rejects malformed appeal responses", async () => {
    const appeal = {
      referenceCode: review.referenceCode,
      reason: "Please reconsider this review.",
      status: "under_appeal",
      createdAt: "2026-11-02T00:00:00Z",
      ignored: "not returned",
    };
    const fetchMock = mockFetch({
      "/api/v1/feedback/status/CE-ABC123/appeal": { status: 201, body: appeal },
    });

    await expect(createAppeal(review.referenceCode, appeal.reason, "student-token")).resolves.toEqual({
      referenceCode: appeal.referenceCode,
      reason: appeal.reason,
      status: appeal.status,
      createdAt: appeal.createdAt,
    });
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({ reason: appeal.reason });
    expect(headersOf(fetchMock)).toHaveProperty("Authorization");

    mockFetch({ "/api/v1/feedback/status/CE-ABC123/appeal": { body: { ...appeal, status: 3 } } });
    await expect(createAppeal(review.referenceCode, appeal.reason)).rejects.toThrow(
      "unexpected appeal response",
    );
    mockFetch({ "/api/v1/feedback/status/CE-ABC123/appeal": { status: 400, body: { error: "appeal already exists" } } });
    await expect(createAppeal(review.referenceCode, appeal.reason)).rejects.toThrow("appeal already exists");
  });

  it("exposes the review category choices used by the submission form", () => {
    expect(reviewCategories.map(({ value }) => value)).toEqual([
      "teaching",
      "content",
      "assessment",
      "workload",
      "resources",
      "other",
    ]);
  });
});
