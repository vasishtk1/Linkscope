import { Suspense } from "react";
import { Linkscope } from "@/components/linkscope";

export default function Home() {
  return (
    <Suspense>
      <Linkscope />
    </Suspense>
  );
}
