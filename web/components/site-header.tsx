import Link from "next/link";

export function SiteHeader({ active }: { active?: "pipeline" }) {
  const link = (on: boolean) => `text-sm transition ${on ? "text-zinc-100" : "text-zinc-400 hover:text-zinc-100"}`;
  return (
    <header className="relative z-10 mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
      <Link href="/" className="flex items-center gap-2 text-sm font-semibold tracking-tight text-zinc-100">
        <Logo />
        Linkscope
      </Link>
      <nav className="flex items-center gap-6">
        <Link href="/pipeline" className={link(active === "pipeline")} aria-current={active === "pipeline" ? "page" : undefined}>
          Pipeline
        </Link>
        <a href="https://github.com/vasishtk1/Linkscope" className={link(false)}>
          GitHub
        </a>
      </nav>
    </header>
  );
}

function Logo() {
  return (
    <svg width="22" height="22" viewBox="0 0 28 28" fill="none" stroke="#60a5fa" strokeWidth="2" aria-hidden="true">
      <circle cx="14" cy="14" r="4" />
      <circle cx="5" cy="6" r="2.5" />
      <circle cx="23" cy="6" r="2.5" />
      <circle cx="22" cy="23" r="2.5" />
      <path d="M7 7.5 11 11.5M21 7.5l-4 4M20.5 21l-3.5-4" />
    </svg>
  );
}
