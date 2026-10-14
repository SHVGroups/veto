import type { Metadata } from "next";
import { WebhooksPage } from "./webhooks";

export const metadata: Metadata = { title: "Webhooks" };

export default function Page() {
  return <WebhooksPage />;
}
