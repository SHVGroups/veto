import type { Metadata } from "next";
import { ToolsPage } from "./tools";

export const metadata: Metadata = { title: "Tools" };

export default function Page() {
  return <ToolsPage />;
}
