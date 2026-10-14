import type { Metadata } from "next";
import { MembersPage } from "./members";

export const metadata: Metadata = { title: "Members" };

export default function Page() {
  return <MembersPage />;
}
