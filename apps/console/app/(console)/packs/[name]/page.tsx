import type { Metadata } from "next";
import { Suspense } from "react";
import { PackView } from "./pack";

export const metadata: Metadata = { title: "Question pack" };

export default async function PackPage({ params }: PageProps<"/packs/[name]">) {
  const { name } = await params;
  return (
    <Suspense>
      <PackView name={name} />
    </Suspense>
  );
}
