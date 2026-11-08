import { ApiUnavailable, network } from "@/lib/api";

// The top sites never change while the API runs, so browsers can keep the answer for an hour.
export async function GET() {
  try {
    const nw = await network(60);
    return Response.json(nw, { headers: { "Cache-Control": "public, max-age=3600" } });
  } catch (err) {
    if (!(err instanceof ApiUnavailable)) throw err;
    console.error(err.message);
    return Response.json({ error: "lookup service unavailable" }, { status: 503 });
  }
}
