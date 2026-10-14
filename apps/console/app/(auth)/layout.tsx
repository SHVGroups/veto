import type { ReactNode } from "react";
import { VetoMark } from "@/components/icons";

/** Sign-in, first-run setup and invitations share this quiet frame. */
export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <main className="flex min-h-dvh flex-col items-center px-4 py-12 sm:justify-center">
      <div className="w-full max-w-[400px]">
        <div className="mb-10 flex items-center gap-2.5 text-ink">
          <VetoMark size={26} />
          <span className="text-lg font-semibold tracking-tight">Veto</span>
        </div>
        {children}
      </div>
    </main>
  );
}
