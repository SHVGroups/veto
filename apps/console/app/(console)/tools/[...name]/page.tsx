import type { Metadata } from "next";
import { ToolEditor } from "./tool";

async function toolName(params: Promise<{ name: string[] }>) {
  const { name } = await params;
  return name.map(decodeURIComponent).join("/");
}

export async function generateMetadata({ params }: PageProps<"/tools/[...name]">): Promise<Metadata> {
  return { title: await toolName(params) };
}

export default async function ToolPage({ params }: PageProps<"/tools/[...name]">) {
  return <ToolEditor name={await toolName(params)} />;
}
