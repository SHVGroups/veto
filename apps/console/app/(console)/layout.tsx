import type { ReactNode } from "react";
import { ConsoleShell } from "@/components/shell/shell";

export default function ConsoleLayout({ children }: { children: ReactNode }) {
  return <ConsoleShell>{children}</ConsoleShell>;
}
