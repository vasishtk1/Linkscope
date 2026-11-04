import { Download } from "lucide-react";

export function DownloadCsv({ domain, count }: { domain: string; count: number }) {
  return (
    <a
      href={`/api/backlinks.csv?domain=${encodeURIComponent(domain)}`}
      download={`${domain}-backlinks.csv`}
      className="flex items-center gap-2 rounded-xl border border-zinc-700 bg-zinc-900 px-5 py-3 text-sm font-medium text-zinc-100 transition hover:border-zinc-500 hover:bg-zinc-800"
    >
      <Download className="size-4" aria-hidden="true" />
      Download backlinks CSV
      <span className="text-zinc-500">({count.toLocaleString()} sites)</span>
    </a>
  );
}
