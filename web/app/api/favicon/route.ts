import type { NextRequest } from "next/server";
import { isKnownDomain } from "@/lib/api";

type Icon = { body: ArrayBuffer; type: string } | null;

const cache = new Map<string, Promise<Icon>>();

export async function GET(req: NextRequest) {
  const domain = req.nextUrl.searchParams.get("domain") ?? "";
  // Only domains from our own data, so this can't be used to make the server fetch arbitrary hosts.
  if (!(await isKnownDomain(domain))) {
    return new Response(null, { status: 404 });
  }
  let icon = cache.get(domain);
  if (!icon) {
    icon = fetchIcon(domain);
    cache.set(domain, icon);
  }
  const found = await icon;
  if (!found) {
    return new Response(null, { status: 404, headers: { "Cache-Control": "public, max-age=86400" } });
  }
  return new Response(found.body, {
    headers: { "Content-Type": found.type, "Cache-Control": "public, max-age=604800" },
  });
}

// fetchIcon tries the site's /favicon.ico, then the www. host, each with a 3s limit.
async function fetchIcon(domain: string): Promise<Icon> {
  for (const host of [domain, `www.${domain}`]) {
    try {
      const res = await fetch(`https://${host}/favicon.ico`, {
        signal: AbortSignal.timeout(3000),
        headers: { "User-Agent": "Linkscope favicon fetcher" },
      });
      const type = res.headers.get("content-type") ?? "";
      if (!res.ok || !type.startsWith("image/")) continue;
      const body = await res.arrayBuffer();
      if (body.byteLength > 0 && body.byteLength < 200_000) return { body, type };
    } catch {
      // Unreachable host or timeout: try the next one, then fall back to a letter badge.
    }
  }
  return null;
}
