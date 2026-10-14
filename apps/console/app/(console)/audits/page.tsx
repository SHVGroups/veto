import type { Metadata } from "next";
import { Audits } from "./audits";

export const metadata: Metadata = { title: "Audits" };

export default function Page() {
  return <Audits />;
}
