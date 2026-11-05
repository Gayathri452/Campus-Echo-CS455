import { vi } from "vitest";

/** Replaces fetch with a mock that answers each URL from `routes`. */
export function mockFetch(routes: Record<string, { status?: number; body?: unknown; raw?: string }>) {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, _init?: RequestInit) => {
    const url = typeof input === "string" ? input : input.toString();
    const path = url.split("?")[0];
    const route = routes[path];
    if (!route) return new Response(JSON.stringify({ error: `no mock for ${url}` }), { status: 404 });
    const status = route.status ?? 200;
    const text = route.raw ?? (route.body === undefined ? null : JSON.stringify(route.body));
    return new Response(status === 204 ? null : text, { status });
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

/** The headers object passed to the n-th fetch call. */
export function headersOf(fetchMock: ReturnType<typeof mockFetch>, call = 0): Record<string, string> {
  const init = fetchMock.mock.calls[call]?.[1] as RequestInit | undefined;
  return (init?.headers ?? {}) as Record<string, string>;
}
