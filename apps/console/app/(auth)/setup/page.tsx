import type { Metadata } from "next";
import { SetupForm } from "./setup-form";

export const metadata: Metadata = { title: "Set up Veto" };

export default function SetupPage() {
  return <SetupForm />;
}
