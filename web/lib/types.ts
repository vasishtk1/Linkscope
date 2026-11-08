export type Backlink = {
  domain: string;
  pages: number;
  nofollowPages: number;
  exampleSource: string;
  exampleTarget: string;
};

export type Lookup = {
  domain: string;
  score: number | null;
  linkingDomains: number;
  linkingPages: number;
  nofollowPages: number;
  backlinks: Backlink[];
  source: string;
};

export type Network = {
  nodes: { domain: string; score: number }[];
  links: { source: string; target: string; pages: number }[];
};

export type WorkerState = "working" | "idle" | "finished" | "gone" | "offline";

export type PipelineWorker = {
  name: string;
  state: WorkerState;
  jobId?: number;
  path?: string;
  jobSeconds?: number;
  lastSeenSeconds: number;
  done: number;
  pages: number;
};

export type FinishedJob = {
  id: number;
  path: string;
  worker: string;
  attempt: number;
  pages: number;
  edges: number;
  durationMs: number;
  finishedAt: string;
};

export type PipelineStatus = {
  pending: number;
  running: number;
  done: number;
  failed: number;
  total: number;
  pages: number;
  crossLinks: number;
  edges: number;
  uptimeSeconds: number;
  jobsPerMinute: number;
  pagesPerSecond: number;
  failures: { id: number; path: string; attempts: number; error: string }[];
  workers: PipelineWorker[];
  now: string;
};

export type Pipeline = { status: PipelineStatus; jobs: FinishedJob[] };
