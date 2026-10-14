"use client";

import { useQueryClient } from "@tanstack/react-query";
import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import type { LiveEvent } from "./types";

export type LiveState = "connecting" | "live" | "offline";

interface LiveContextValue {
  state: LiveState;
  /** The most recent events, newest first. */
  recent: LiveEvent[];
}

const LiveContext = createContext<LiveContextValue>({ state: "offline", recent: [] });

const KINDS = [
  "action.received",
  "action.held",
  "action.decided",
  "outcome.reported",
  "audit.sampled",
  "audit.recorded",
  "tool.registered",
  "config.changed",
];

/**
 * Subscribes to the workspace's event stream and keeps the console current.
 * Events only name what changed; affected queries are refetched, so the
 * stream never becomes a second source of truth. EventSource reconnects on
 * its own using the server's retry hint.
 */
export function LiveProvider({ children, enabled }: { children: ReactNode; enabled: boolean }) {
  const qc = useQueryClient();
  const [state, setState] = useState<LiveState>("connecting");
  const [recent, setRecent] = useState<LiveEvent[]>([]);
  const pending = useRef(new Set<string>());
  const timer = useRef<number | null>(null);

  useEffect(() => {
    if (!enabled) return;
    const es = new EventSource("/v1/events");

    // Coalesce bursts: many actions arriving at once refetch each family once.
    const invalidate = (family: string) => {
      pending.current.add(family);
      if (timer.current !== null) return;
      timer.current = window.setTimeout(() => {
        for (const f of pending.current) void qc.invalidateQueries({ queryKey: [f] });
        pending.current.clear();
        timer.current = null;
      }, 400);
    };

    const onEvent = (msg: MessageEvent<string>) => {
      let e: LiveEvent;
      try {
        e = JSON.parse(msg.data) as LiveEvent;
      } catch {
        return;
      }
      setRecent((r) => [e, ...r].slice(0, 60));
      if (e.action_id) void qc.invalidateQueries({ queryKey: ["action", e.action_id] });
      switch (e.kind) {
        case "action.received":
        case "action.held":
        case "action.decided":
          invalidate("actions");
          invalidate("overview");
          invalidate("agents");
          break;
        case "outcome.reported":
          invalidate("overview");
          invalidate("actions");
          break;
        case "audit.sampled":
        case "audit.recorded":
          invalidate("audits");
          invalidate("calibration");
          invalidate("overview");
          break;
        case "tool.registered":
          invalidate("tools");
          invalidate("overview");
          break;
        case "config.changed":
          invalidate("tools");
          invalidate("policies");
          invalidate("packs");
          break;
      }
    };

    es.onopen = () => setState("live");
    es.onerror = () => setState(es.readyState === EventSource.CLOSED ? "offline" : "connecting");
    for (const k of KINDS) es.addEventListener(k, onEvent as EventListener);

    return () => {
      es.close();
      if (timer.current !== null) window.clearTimeout(timer.current);
      timer.current = null;
    };
  }, [enabled, qc]);

  return <LiveContext.Provider value={{ state: enabled ? state : "offline", recent }}>{children}</LiveContext.Provider>;
}

export function useLive(): LiveContextValue {
  return useContext(LiveContext);
}
