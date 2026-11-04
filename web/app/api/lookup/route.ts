import type { NextRequest } from "next/server";
import { lookup } from "@/lib/edges";

export async function GET(req: NextRequest) {
  const q = req.nextUrl.searchParams.get("domain") ?? "";
  if (!q.trim()) {
    return Response.json({ error: "missing domain" }, { status: 400 });
  }
  const result = await lookup(q);
  if (!result) {
    return Response.json({ error: "no backlinks found" }, { status: 404 });
  }
  return Response.json(result);
}
