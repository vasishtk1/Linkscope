import type { NextRequest } from "next/server";
import { lookup } from "@/lib/api";

export async function GET(req: NextRequest) {
  const result = await lookup(req.nextUrl.searchParams.get("domain") ?? "", 0);
  if (!result) {
    return new Response("no backlinks found\n", { status: 404 });
  }
  const rows = [["linking_domain", "pages", "nofollow_pages", "example_source", "example_target"]];
  for (const b of result.backlinks) {
    rows.push([b.domain, String(b.pages), String(b.nofollowPages), b.exampleSource, b.exampleTarget]);
  }
  const csv = rows.map((r) => r.map(csvField).join(",")).join("\n") + "\n";
  return new Response(csv, {
    headers: {
      "Content-Type": "text/csv; charset=utf-8",
      "Content-Disposition": `attachment; filename="${result.domain}-backlinks.csv"`,
    },
  });
}

// csvField quotes a value when it contains a comma, quote or newline.
function csvField(v: string): string {
  return /[",\n]/.test(v) ? `"${v.replaceAll('"', '""')}"` : v;
}
