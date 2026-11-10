import { useSyncExternalStore } from "react";
import type { Lookup, Origin } from "./types";

// SavedSearch is what the pipeline page needs to replay one search, kept in this browser only.
export type SavedSearch = {
  domain: string;
  score: number | null;
  linkingDomains: number;
  linkingPages: number;
  originFiles: number;
  origins: Origin[];
  tookMs: number;
  source: string;
  at: number;
};

const KEY = "linkscope.searches.v1";
const MAX = 20;
const CHANGED = "linkscope-searches";
const EMPTY: SavedSearch[] = [];

let cache: { raw: string | null; list: SavedSearch[] } = { raw: null, list: EMPTY };

function read(): string | null {
  try {
    return localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

function valid(x: unknown): x is SavedSearch {
  const s = x as SavedSearch;
  return !!s && typeof s.domain === "string" && typeof s.linkingPages === "number" && typeof s.at === "number" && Array.isArray(s.origins);
}

// parse turns the stored text into a list; anything broken or unexpected becomes an empty list.
export function parse(raw: string | null): SavedSearch[] {
  if (!raw) return EMPTY;
  try {
    const v: unknown = JSON.parse(raw);
    return Array.isArray(v) ? v.filter(valid).slice(0, MAX) : EMPTY;
  } catch {
    return EMPTY;
  }
}

// addSearch puts a search at the top of the list, replacing an older search for the same site.
export function addSearch(list: SavedSearch[], s: SavedSearch): SavedSearch[] {
  return [s, ...list.filter((x) => x.domain !== s.domain)].slice(0, MAX);
}

function snapshot(): SavedSearch[] {
  const raw = read();
  if (raw !== cache.raw) cache = { raw, list: parse(raw) };
  return cache.list;
}

function subscribe(onChange: () => void) {
  const onStorage = (e: StorageEvent) => e.key === KEY && onChange();
  window.addEventListener("storage", onStorage);
  window.addEventListener(CHANGED, onChange);
  return () => {
    window.removeEventListener("storage", onStorage);
    window.removeEventListener(CHANGED, onChange);
  };
}

export function useSearches(): SavedSearch[] {
  return useSyncExternalStore(subscribe, snapshot, () => EMPTY);
}

// saveSearch remembers a found site on this device; it does nothing if storage is blocked.
export function saveSearch(l: Lookup) {
  const s: SavedSearch = {
    domain: l.domain,
    score: l.score,
    linkingDomains: l.linkingDomains,
    linkingPages: l.linkingPages,
    originFiles: l.originFiles ?? 0,
    origins: l.origins ?? [],
    tookMs: l.tookMs ?? 0,
    source: l.source,
    at: Date.now(),
  };
  try {
    localStorage.setItem(KEY, JSON.stringify(addSearch(snapshot(), s)));
    window.dispatchEvent(new Event(CHANGED));
  } catch {}
}

export function clearSearches() {
  try {
    localStorage.removeItem(KEY);
    window.dispatchEvent(new Event(CHANGED));
  } catch {}
}
