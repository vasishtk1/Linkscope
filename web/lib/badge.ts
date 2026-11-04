// badgeColor picks a stable color for a domain's letter badge when it has no favicon.
export function badgeColor(domain: string): string {
  let h = 0;
  for (let i = 0; i < domain.length; i++) h = (h * 31 + domain.charCodeAt(i)) >>> 0;
  return `hsl(${h % 360} 45% 38%)`;
}

export function faviconUrl(domain: string): string {
  return `/api/favicon?domain=${encodeURIComponent(domain)}`;
}
