import { ApiUnavailable, savedRun } from "@/lib/api";

export async function GET() {
  try {
    const run = await savedRun();
    if (!run) return Response.json({ error: "no saved run" }, { status: 404 });
    return Response.json(run);
  } catch (err) {
    if (!(err instanceof ApiUnavailable)) throw err;
    return Response.json({ error: "lookup service unavailable" }, { status: 503 });
  }
}
