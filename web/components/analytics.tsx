"use client";

import { Analytics as VercelAnalytics, type BeforeSendEvent } from "@vercel/analytics/next";

const KEY = "linkscope-notrack";

// skipOwnVisits drops visits from a browser that opened the site once with ?notrack=1 (?notrack=0 undoes it).
function skipOwnVisits(event: BeforeSendEvent) {
  try {
    const url = new URL(event.url);
    const flag = url.searchParams.get("notrack");
    if (flag === "1") localStorage.setItem(KEY, "1");
    if (flag === "0") localStorage.removeItem(KEY);
    if (localStorage.getItem(KEY) === "1") return null;
    url.searchParams.delete("notrack");
    return { ...event, url: url.toString() };
  } catch {
    return event;
  }
}

export function Analytics() {
  return <VercelAnalytics beforeSend={skipOwnVisits} />;
}
