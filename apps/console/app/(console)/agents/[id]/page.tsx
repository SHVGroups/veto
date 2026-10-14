import type { Metadata } from "next";
import { AgentView } from "./agent";

export const metadata: Metadata = { title: "Agent" };

export default async function AgentPage({ params }: PageProps<"/agents/[id]">) {
  const { id } = await params;
  return <AgentView id={id} />;
}
