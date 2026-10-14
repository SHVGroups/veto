import type { Metadata } from "next";
import { SlackPage } from "./slack";

export const metadata: Metadata = { title: "Slack" };

export default function Page() {
  return <SlackPage />;
}
