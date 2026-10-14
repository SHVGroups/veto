"use client";

import { useCallback, useSyncExternalStore } from "react";

/*
 * Small per-browser preferences kept in localStorage, read without a
 * hydration mismatch: the server and the first client render use the
 * default, then the stored value applies.
 */

const EVENT = "veto:stored";

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function subscribe(cb: () => void) {
  window.addEventListener(EVENT, cb);
  window.addEventListener("storage", cb);
  return () => {
    window.removeEventListener(EVENT, cb);
    window.removeEventListener("storage", cb);
  };
}

/** A string preference limited to known values, with a setter that persists it. */
export function useStored<T extends string>(key: string, fallback: T, allowed: readonly T[]): [T, (v: T | null) => void] {
  const raw = useSyncExternalStore(
    subscribe,
    () => read(key),
    () => null,
  );
  const value = raw !== null && (allowed as readonly string[]).includes(raw) ? (raw as T) : fallback;
  const set = useCallback(
    (v: T | null) => {
      try {
        if (v === null) localStorage.removeItem(key);
        else localStorage.setItem(key, v);
      } catch {
        // Storage can be unavailable in private windows; the choice is not remembered.
      }
      window.dispatchEvent(new Event(EVENT));
    },
    [key],
  );
  return [value, set];
}
