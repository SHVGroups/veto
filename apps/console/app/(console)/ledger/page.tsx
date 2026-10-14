import type { Metadata } from "next";
import { Suspense } from "react";
import { LedgerPage } from "./ledger";

export const metadata: Metadata = { title: "Ledger" };

export default function Page() {
  return (
    <Suspense>
      <LedgerPage />
    </Suspense>
  );
}
