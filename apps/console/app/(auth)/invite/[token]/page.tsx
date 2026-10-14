import type { Metadata } from "next";
import { AcceptInvitation } from "./accept";

export const metadata: Metadata = { title: "Join workspace" };

export default async function InvitePage({ params }: PageProps<"/invite/[token]">) {
  const { token } = await params;
  return <AcceptInvitation token={token} />;
}
