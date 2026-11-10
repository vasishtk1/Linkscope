export type Backlink = {
  domain: string;
  pages: number;
  nofollowPages: number;
  exampleSource: string;
  exampleTarget: string;
};

// Origin is one Common Crawl file that had links to the site, matched to the replay by file name.
export type Origin = { file: string; pages: number };

export type Lookup = {
  domain: string;
  score: number | null;
  linkingDomains: number;
  linkingPages: number;
  nofollowPages: number;
  backlinks: Backlink[];
  source: string;
  originFiles?: number;
  origins?: Origin[];
  tookMs?: number;
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

export type RunEvent = {
  id: number;
  at: string;
  job: number;
  path: string;
  worker: string;
  kind: "claimed" | "completed" | "failed" | "expired" | "released";
  attempt: number;
  final?: boolean;
  pages?: number;
  crossLinks?: number;
  edges?: number;
  durationMs?: number;
  error?: string;
};

export type PipelineStatus = {
  pending: number;
  running: number;
  done: number;
  failed: number;
  total: number;
  workers: PipelineWorker[];
  now: string;
};

// Pipeline is the live coordinator: its status and its whole event log.
export type Pipeline = { status: PipelineStatus; events: RunEvent[] };

// Recording is a saved run from cmd/record.
export type Recording = {
  name: string;
  recordedAt: string;
  derived: boolean;
  files: { id: number; path: string; state: string; attempts: number; pages: number; crossLinks: number; edges: number }[];
  workers: string[];
  events: RunEvent[];
  totals: { files: number; done: number; failed: number; pages: number; crossLinks: number; edges: number; failedAttempts: number; seconds: number };
};
