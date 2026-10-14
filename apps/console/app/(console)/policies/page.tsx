import type { Metadata } from "next";
import { PoliciesPage } from "./policies";

export const metadata: Metadata = { title: "Policies" };

export default function Page() {
  return <PoliciesPage />;
}
