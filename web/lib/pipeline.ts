import "server-only";
import type { Pipeline, PipelineStatus, RunEvent } from "./types";

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

// pipeline asks the coordinator for its live status and its event log, in parallel.
export async function pipeline(): Promise<Pipeline> {
  const [status, log] = await Promise.all([
    get<PipelineStatus>("/status"),
    get<{ events: RunEvent[] }>("/events"),
  ]);
  return { status, events: log.events };
}
