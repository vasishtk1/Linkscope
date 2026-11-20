import cardplayer from "./agent-runs/cardplayer.json";
import fansided from "./agent-runs/fansided.json";

export type Step = { at: number; step: string; page?: string; text: string; thinking?: string };

export type Problem = {
  page: string;
  result: string;
  status: number;
  sites: number;
  error?: string;
  suggestion?: {
    target?: string;
    by?: string;
    pick?: { path?: string; score?: number; shared?: string[] };
    noMatch?: string;
    ai?: { choice?: string; confidence?: string; reason?: string; model?: string };
  };
};

export type PullRequest = {
  repo: string;
  number: number;
  url: string;
  title: string;
  branch: string;
  author: string;
  opened: string;
  steps: { at?: number; text: string }[];
};

export type Run = {
  site: string;
  recorded: string;
  tookMs: number;
  checked: number;
  blocked: number;
  problems: Problem[];
  steps: Step[];
  file: { path: string; before: string; after: string };
  pr?: PullRequest;
};

export const RUNS: Record<string, Run> = { "fansided.com": fansided as Run, "cardplayer.com": cardplayer as Run };
export const DEFAULT_SITE = "fansided.com";

// Stages of the agent, in order, and which trace steps belong to each.
export const STAGES = [
  { id: "check", label: "Check links", steps: ["start", "check"] },
  { id: "match", label: "Match addresses", steps: ["sitemap", "match"] },
  { id: "ask", label: "Ask Gemini if unclear", steps: ["archive", "read", "ask", "decide"] },
  { id: "edit", label: "Edit redirect file", steps: ["done"] },
  { id: "pr", label: "Open pull request", steps: ["github"] },
] as const;

// prSchedule spaces the pull request steps at their real pace; a step without a recorded time goes halfway between its neighbors.
export function prSchedule(steps: { at?: number }[]) {
  return steps.map((s, i) => {
    if (s.at !== undefined) return s.at;
    const before = steps.slice(0, i).reverse().find((x) => x.at !== undefined)?.at ?? 0;
    const after = steps.slice(i + 1).find((x) => x.at !== undefined)?.at ?? before + 1;
    return (before + after) / 2;
  });
}

export function stageOf(step: string) {
  return STAGES.findIndex((s) => (s.steps as readonly string[]).includes(step));
}

const MAX_GAP = 2.2;
const MIN_GAP = 0.3;
export const TYPE_RATE = 260;

// schedule turns real trace times into replay times: short gaps keep their pace, long waits are shortened.
export function schedule(steps: Step[]) {
  const times: number[] = [];
  let t = 0.4;
  steps.forEach((s, i) => {
    if (i > 0) {
      const prev = steps[i - 1];
      const gap = s.at - prev.at;
      t += Math.min(MAX_GAP, Math.max(MIN_GAP, gap * 0.6));
      if (prev.thinking) t += prev.thinking.length / TYPE_RATE;
    }
    times.push(t);
  });
  return times;
}

// realTime maps a replay time back to the run's own clock, so the timer shows the agent's real pace.
export function realTime(steps: Step[], times: number[], now: number) {
  if (steps.length === 0) return 0;
  let i = 0;
  while (i + 1 < times.length && times[i + 1] <= now) i++;
  if (i + 1 >= times.length || now <= times[0]) return now <= times[0] ? 0 : steps[steps.length - 1].at;
  const f = (now - times[i]) / (times[i + 1] - times[i]);
  return steps[i].at + f * (steps[i + 1].at - steps[i].at);
}

export type DiffLine = { kind: "same" | "add" | "del"; text: string };

// diffLines is a small longest-common-subsequence line diff, enough for a config file.
export function diffLines(before: string, after: string): DiffLine[] {
  const a = before.replace(/\n$/, "").split("\n");
  const b = after.replace(/\n$/, "").split("\n");
  const n = a.length;
  const m = b.length;
  const dp = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0));
  for (let i = n - 1; i >= 0; i--)
    for (let j = m - 1; j >= 0; j--) dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1]);
  const out: DiffLine[] = [];
  let i = 0;
  let j = 0;
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      out.push({ kind: "same", text: a[i] });
      i++;
      j++;
    } else if (dp[i + 1][j] >= dp[i][j + 1]) out.push({ kind: "del", text: a[i++] });
    else out.push({ kind: "add", text: b[j++] });
  }
  while (i < n) out.push({ kind: "del", text: a[i++] });
  while (j < m) out.push({ kind: "add", text: b[j++] });
  return out;
}

// pathOf shows a page as its path on the main site, or host plus path on a subdomain.
export function pathOf(page: string, site: string) {
  const slash = page.indexOf("/");
  const host = slash < 0 ? page : page.slice(0, slash);
  const path = slash < 0 ? "/" : page.slice(slash);
  return host === site ? path : host + (path === "/" ? "" : path);
}
