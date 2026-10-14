import type { Metadata } from "next";
import { Suspense } from "react";
import { ActionsExplorer } from "./explorer";

export const metadata: Metadata = { title: "Actions" };

export default function ActionsPage() {
  return (
    <Suspense>
      <ActionsExplorer />
    </Suspense>
  );
}
