import type { Metadata } from "next";
import { Suspense } from "react";
import { ReviewQueue } from "./queue";

export const metadata: Metadata = { title: "Review" };

export default function ReviewPage() {
  return (
    <Suspense>
      <ReviewQueue />
    </Suspense>
  );
}
