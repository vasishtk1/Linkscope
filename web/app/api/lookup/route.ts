import type { NextRequest } from "next/server";
import { ApiUnavailable, lookup } from "@/lib/api";

export async function GET(req: NextRequest) {
  const q = req.nextUrl.searchParams.get("domain") ?? "";
  if (!q.trim()) {
    return Response.json({ error: "missing domain" }, { status: 400 });
  }
  let result;
  try {
    result = await lookup(q);
  } catch (err) {
    if (!(err instanceof ApiUnavailable)) throw err;
    console.error(err.message);
    return Response.json({ error: "lookup service unavailable" }, { status: 503 });
  }
  if (!result) {
    return Response.json({ error: "no backlinks found" }, { status: 404 });
  }
  return Response.json(result);
}
