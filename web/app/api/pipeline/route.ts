import { CoordinatorUnavailable, pipeline } from "@/lib/pipeline";

export async function GET() {
  try {
    return Response.json(await pipeline());
  } catch (err) {
    if (!(err instanceof CoordinatorUnavailable)) throw err;
    return Response.json({ error: "no pipeline running" }, { status: 503 });
  }
}
