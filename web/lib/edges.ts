import "server-only";
import { readFile } from "node:fs/promises";
import path from "node:path";
import type { Backlink, Lookup } from "./types";

const EDGES_FILE =
  process.env.LINKSCOPE_EDGES ?? path.join(process.cwd(), "..", "data", "sample.edges.tsv");

const SOURCE = "Sample: 1 Common Crawl file from CC-MAIN-2026-43 (22,020 pages)";

type Index = { byTarget: Map<string, Backlink[]>; domains: Set<string> };

let index: Promise<Index> | undefined;

// loadIndex reads the edge file once and groups backlinks by the domain they point to.
export function loadIndex(): Promise<Index> {
  index ??= readFile(/*turbopackIgnore: true*/ EDGES_FILE, "utf8").then(buildIndex);
  return index;
}

function buildIndex(tsv: string): Index {
  const byTarget = new Map<string, Backlink[]>();
  const domains = new Set<string>();
  const lines = tsv.split("\n");
  for (let i = 1; i < lines.length; i++) {
    const [src, dst, pages, nofollow, exampleSource, exampleTarget] = lines[i].split("\t");
    if (!dst) continue;
    domains.add(src);
    domains.add(dst);
    let list = byTarget.get(dst);
    if (!list) byTarget.set(dst, (list = []));
    list.push({
      domain: src,
      pages: Number(pages),
      nofollowPages: Number(nofollow),
      exampleSource,
      exampleTarget,
    });
  }
  for (const list of byTarget.values()) {
    list.sort((a, b) => b.pages - a.pages || a.domain.localeCompare(b.domain));
  }
  return { byTarget, domains };
}

// normalizeInput turns whatever the user typed ("https://www.Example.com/page") into a bare host.
export function normalizeInput(input: string): string {
  let s = input.trim().toLowerCase();
  s = s.replace(/^[a-z][a-z0-9+.-]*:\/\//, "");
  s = s.split(/[/?#]/)[0].split("@").pop()!.split(":")[0];
  return s.replace(/\.$/, "").replace(/^www\./, "");
}

// resolveDomain finds the indexed domain for a host, dropping subdomains until one matches.
export function resolveDomain(host: string, known: Map<string, unknown>): string | undefined {
  const labels = host.split(".");
  for (let i = 0; i < labels.length - 1; i++) {
    const candidate = labels.slice(i).join(".");
    if (known.has(candidate)) return candidate;
  }
  return undefined;
}

export async function lookup(input: string): Promise<Lookup | undefined> {
  const { byTarget } = await loadIndex();
  const domain = resolveDomain(normalizeInput(input), byTarget);
  if (!domain) return undefined;
  const backlinks = byTarget.get(domain)!;
  let pages = 0;
  let nofollow = 0;
  for (const b of backlinks) {
    pages += b.pages;
    nofollow += b.nofollowPages;
  }
  return {
    domain,
    score: null,
    linkingDomains: backlinks.length,
    linkingPages: pages,
    nofollowPages: nofollow,
    backlinks,
    source: SOURCE,
  };
}

export async function isKnownDomain(domain: string): Promise<boolean> {
  return (await loadIndex()).domains.has(domain);
}
