import type { Metadata } from "next";
import { PacksPage } from "./packs";

export const metadata: Metadata = { title: "Question packs" };

export default function Page() {
  return <PacksPage />;
}
