"use client";

import type { Route } from "next";
import { usePathname, useRouter } from "next/navigation";
import { createContext, useContext, useEffect, type ReactNode } from "react";
import { ApiError } from "@/lib/api";
import { can, type Permission } from "@/lib/permissions";
import { useMe, useSetupState } from "@/lib/queries";
import type { Me } from "@/lib/types";
import { VetoMark } from "../icons";
import { ErrorState } from "../ui/states";

const SessionContext = createContext<Me | null>(null);

/**
 * Loads the signed-in member and sends anyone without a session to sign in,
 * or to first-run setup on a new deployment. The server enforces access on
 * every request; this only decides what to render.
 */
export function SessionGate({ children }: { children: ReactNode }) {
  const router = useRouter();
  const pathname = usePathname();
  const me = useMe({ retry: false });
  const unauthorized = me.error instanceof ApiError && me.error.status === 401;
  const setup = useSetupState();

  useEffect(() => {
    if (!unauthorized || setup.isPending) return;
    if (setup.data?.needs_setup) router.replace("/setup");
    else router.replace(`/login?next=${encodeURIComponent(pathname)}` as Route);
  }, [unauthorized, setup.isPending, setup.data, router, pathname]);

  if (me.data) return <SessionContext.Provider value={me.data}>{children}</SessionContext.Provider>;

  if (me.isError && !unauthorized) {
    return (
      <div className="mx-auto mt-24 max-w-lg px-4">
        <ErrorState error={me.error} onRetry={() => void me.refetch()} />
      </div>
    );
  }

  return (
    <div className="flex min-h-dvh items-center justify-center text-ink-3" aria-busy="true">
      <VetoMark size={28} className="animate-[veto-pulse_1.6s_ease-in-out_infinite]" />
    </div>
  );
}

export function useSession(): Me {
  const me = useContext(SessionContext);
  if (!me) throw new Error("useSession must be used inside SessionGate");
  return me;
}

/** Whether the signed-in member's role allows something. */
export function useCan(p: Permission): boolean {
  return can(useSession().role, p);
}
