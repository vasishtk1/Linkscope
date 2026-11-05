import "server-only";
import type { Lookup } from "./types";

const API = process.env.LINKSCOPE_API ?? "http://localhost:8081";

// Enough for the graph and table; the CSV download asks for every backlink.
export const PAGE_LIMIT = 1000;

export class ApiUnavailable extends Error {}

async function call(path: string): Promise<Response> {
  try {
    return await fetch(`${API}${path}`, { cache: "no-store", signal: AbortSignal.timeout(10_000) });
  } catch (err) {
    throw new ApiUnavailable(`lookup API at ${API} unreachable: ${err}`);
  }
}

// lookup asks the Go API, which cleans up the input; undefined means nothing links to the site.
export async function lookup(input: string, limit = PAGE_LIMIT): Promise<Lookup | undefined> {
  const res = await call(`/lookup?domain=${encodeURIComponent(input)}&limit=${limit}`);
  if (res.status === 404) return undefined;
  if (!res.ok) throw new ApiUnavailable(`lookup API returned ${res.status}`);
  return res.json();
}

export async function isKnownDomain(domain: string): Promise<boolean> {
  if (!domain) return false;
  const res = await call(`/known?domain=${encodeURIComponent(domain)}`);
  return res.ok;
}
