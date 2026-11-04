"use client";

import { useState } from "react";
import { ArrowRight, LoaderCircle, Search } from "lucide-react";

export function SearchBar({
  initial,
  onSearch,
  busy,
}: {
  initial: string;
  onSearch: (q: string) => void;
  busy: boolean;
}) {
  const [value, setValue] = useState(initial);

  return (
    <form
      onSubmit={(e) => {
        e.preventDefault();
        if (value.trim()) onSearch(value);
      }}
      className="group flex items-center gap-2 rounded-2xl border border-zinc-800 bg-zinc-900/70 p-2 pl-4 shadow-[0_0_0_1px_rgba(0,0,0,0.4),0_20px_60px_-20px_rgba(59,130,246,0.25)] backdrop-blur transition focus-within:border-zinc-600"
    >
      <Search className="size-5 shrink-0 text-zinc-500" aria-hidden="true" />
      <label htmlFor="site" className="sr-only">
        Website link
      </label>
      <input
        id="site"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        placeholder="Paste a website link, like github.com"
        autoComplete="off"
        spellCheck={false}
        className="h-11 min-w-0 flex-1 bg-transparent text-base text-zinc-100 placeholder:text-zinc-500 focus:outline-none"
      />
      <button
        type="submit"
        disabled={busy}
        className="flex h-11 items-center gap-2 rounded-xl bg-blue-500 px-5 text-sm font-medium text-white transition hover:bg-blue-400 disabled:opacity-70"
      >
        {busy ? (
          <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
        ) : (
          <ArrowRight className="size-4" aria-hidden="true" />
        )}
        Look up
      </button>
    </form>
  );
}
