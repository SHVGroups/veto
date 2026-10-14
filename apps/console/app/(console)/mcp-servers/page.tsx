import type { Metadata } from "next";
import { McpPage } from "./mcp";

export const metadata: Metadata = { title: "MCP servers" };

export default function Page() {
  return <McpPage />;
}
