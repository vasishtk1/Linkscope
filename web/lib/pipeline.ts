import "server-only";
import type { FinishedJob, Pipeline, PipelineStatus } from "./types";

const COORDINATOR = process.env.LINKSCOPE_COORDINATOR ?? "http://localhost:8080";

export class CoordinatorUnavailable extends Error {}

async function get<T>(path: string): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${COORDINATOR}${path}`, { cache: "no-store", signal: AbortSignal.timeout(3_000) });
  } catch (err) {
    throw new CoordinatorUnavailable(`coordinator at ${COORDINATOR} unreachable: ${err}`);
  }
  if (!res.ok) throw new CoordinatorUnavailable(`coordinator returned ${res.status} for ${path}`);
  return res.json();
}

// pipeline asks the coordinator for its live status and every finished job, in parallel.
export async function pipeline(): Promise<Pipeline> {
  const [status, timeline] = await Promise.all([
    get<PipelineStatus>("/status"),
    get<{ jobs: FinishedJob[] }>("/timeline"),
  ]);
  return { status, jobs: timeline.jobs };
}
