import type { Metadata } from "next";
import { Agent } from "@/components/agent";
import { SiteHeader } from "@/components/site-header";

export const metadata: Metadata = {
  title: "Agent · Linkscope",
  description: "Watch the Linkscope agent find broken pages that still get backlinks and propose redirects.",
};

export default function AgentPage() {
  return (
    <div className="relative min-h-screen overflow-x-hidden">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 h-[600px] bg-[radial-gradient(ellipse_at_top,rgba(59,130,246,0.12),transparent_60%)]"
      />
      <SiteHeader active="agent" />
      <Agent />
    </div>
  );
}
