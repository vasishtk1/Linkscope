import { Suspense } from "react";
import type { Metadata } from "next";
import { Pipeline } from "@/components/pipeline";
import { SiteHeader } from "@/components/site-header";

export const metadata: Metadata = {
  title: "Pipeline · Linkscope",
  description: "Live view of the Linkscope pipeline: Common Crawl files moving through the coordinator and workers.",
};

export default function PipelinePage() {
  return (
    <div className="relative min-h-screen overflow-x-hidden">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 h-[600px] bg-[radial-gradient(ellipse_at_top,rgba(59,130,246,0.12),transparent_60%)]"
      />
      <SiteHeader active="pipeline" />
      <Suspense>
        <Pipeline />
      </Suspense>
    </div>
  );
}
