import type { Metadata } from "next";
import { ActionDetailView } from "./detail";

export async function generateMetadata({ params }: PageProps<"/actions/[id]">): Promise<Metadata> {
  const { id } = await params;
  return { title: `Action ${id.slice(-6)}` };
}

export default async function ActionPage({ params }: PageProps<"/actions/[id]">) {
  const { id } = await params;
  return <ActionDetailView id={id} />;
}
