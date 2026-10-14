"use client";

import { useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import type { Route } from "next";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState, type ReactNode } from "react";
import { api } from "@/lib/api";
import { cx } from "@/lib/cx";
import { roleLabel } from "@/lib/format";
import { LiveProvider, useLive } from "@/lib/live";
import { useAudits, useQueue } from "@/lib/queries";
import { useTheme, type ThemeChoice } from "@/lib/theme";
import { Icon, VetoMark } from "../icons";
import { Kbd } from "../ui/layout";
import { LiveDot } from "../ui/pills";
import { Menu, MenuItem } from "../ui/table";
import { CommandMenu } from "./command";
import { NAV, isActive } from "./nav";
import { SessionGate, useSession } from "./session";

export function ConsoleShell({ children }: { children: ReactNode }) {
  return (
    <SessionGate>
      <LiveProvider enabled>
        <Frame>{children}</Frame>
      </LiveProvider>
    </SessionGate>
  );
}

function Frame({ children }: { children: ReactNode }) {
  const [drawer, setDrawer] = useState(false);
  const [command, setCommand] = useState(false);

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setCommand((c) => !c);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return (
    <div className="min-h-dvh lg:grid lg:grid-cols-[248px_1fr]">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:rounded-[10px] focus:bg-surface focus:px-3 focus:py-2"
      >
        Skip to content
      </a>
      <MobileBar onMenu={() => setDrawer(true)} onCommand={() => setCommand(true)} />
      {drawer && <div className="fixed inset-0 z-30 bg-[rgb(10_11_13/0.4)] lg:hidden" onClick={() => setDrawer(false)} />}
      <aside
        className={cx(
          "fixed inset-y-0 left-0 z-40 flex w-[264px] flex-col bg-canvas transition-transform duration-200 lg:sticky lg:top-0 lg:h-dvh lg:w-auto lg:translate-x-0",
          drawer ? "translate-x-0 shadow-pop" : "-translate-x-full",
        )}
      >
        <Sidebar onCommand={() => setCommand(true)} onNavigate={() => setDrawer(false)} />
      </aside>
      <main id="main" className="min-w-0 px-4 pt-6 pb-16 sm:px-8 lg:px-10 lg:pt-9">
        <div className="mx-auto max-w-[1320px]">{children}</div>
      </main>
      <CommandMenu open={command} onClose={() => setCommand(false)} />
      <TitleCount />
    </div>
  );
}

function Sidebar({ onCommand, onNavigate }: { onCommand: () => void; onNavigate: () => void }) {
  const pathname = usePathname();
  const me = useSession();
  const live = useLive();
  const queue = useQueue();
  const audits = useAudits("open");
  const counts = { queue: queue.data?.items.length ?? 0, audits: audits.data?.items.length ?? 0 };

  return (
    <div className="flex h-full flex-col px-3 pt-5 pb-3">
      <div className="flex items-center justify-between px-2.5">
        <Link href="/" className="flex items-center gap-2.5 text-ink">
          <VetoMark size={22} />
          <span className="text-[15px] font-semibold tracking-tight">Veto</span>
        </Link>
        <span
          className="flex items-center gap-2 text-xs text-ink-3"
          title={live.state === "live" ? "Receiving live updates" : "Reconnecting to live updates"}
        >
          <LiveDot state={live.state} />
          {live.state === "live" ? "Live" : live.state === "connecting" ? "Connecting" : "Offline"}
        </span>
      </div>
      <div className="mt-1 truncate px-2.5 text-xs text-ink-3">{me.workspace.name}</div>

      <button
        type="button"
        onClick={onCommand}
        className="mt-5 flex h-9 items-center gap-2 rounded-[10px] bg-surface px-3 text-sm text-ink-3 shadow-[inset_0_0_0_1px_var(--line)] hover:text-ink"
      >
        <Icon.Search size={15} />
        <span className="flex-1 text-left">Go to</span>
        <Kbd>⌘K</Kbd>
      </button>

      <nav className="scroll-quiet mt-4 -mr-1 flex-1 overflow-y-auto pr-1" aria-label="Main">
        {NAV.map((g) => (
          <div key={g.group} className="mb-4">
            <div className="px-2.5 pb-1 text-xs text-ink-3">{g.group}</div>
            <ul>
              {g.items.map((item) => {
                const active = isActive(item.href, pathname);
                const I = Icon[item.icon];
                const n = item.badge ? counts[item.badge] : 0;
                return (
                  <li key={item.href}>
                    <Link
                      href={item.href as Route}
                      aria-current={active ? "page" : undefined}
                      onClick={onNavigate}
                      className={cx(
                        "flex h-9 items-center gap-2.5 rounded-[10px] px-2.5 text-sm transition-colors",
                        active
                          ? "bg-surface font-medium text-ink shadow-[0_1px_2px_rgb(0_0_0/0.05)]"
                          : "text-ink-2 hover:bg-hover hover:text-ink",
                      )}
                    >
                      <I size={17} className={active ? "text-ink" : "text-ink-3"} />
                      <span className="flex-1">{item.label}</span>
                      {n > 0 && (
                        <span
                          className={cx(
                            "figures inline-flex h-5 min-w-5 items-center justify-center rounded-full px-1.5 text-2xs font-semibold",
                            item.badge === "queue" ? "bg-held text-white" : "bg-hover text-ink-2",
                          )}
                        >
                          {n > 99 ? "99+" : n}
                        </span>
                      )}
                    </Link>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </nav>

      <UserMenu />
    </div>
  );
}

function UserMenu() {
  const me = useSession();
  const router = useRouter();
  const qc = useQueryClient();
  const { theme, setTheme } = useTheme();
  const initials = me.user.name
    .split(/\s+/)
    .map((p) => p[0])
    .slice(0, 2)
    .join("")
    .toUpperCase();

  async function signOut() {
    try {
      await api.post("/v1/auth/logout");
    } finally {
      qc.clear();
      router.replace("/login");
    }
  }

  const themes: { value: ThemeChoice; label: string; icon: ReactNode }[] = [
    { value: "system", label: "Match system", icon: <Icon.Monitor size={15} /> },
    { value: "light", label: "Light", icon: <Icon.Sun size={15} /> },
    { value: "dark", label: "Dark", icon: <Icon.Moon size={15} /> },
  ];

  return (
    <div className="border-t border-line pt-3">
      <Menu
        align="start"
        trigger={(p) => (
          <button {...p} type="button" className="flex w-full items-center gap-3 rounded-[10px] px-2 py-2 text-left hover:bg-hover">
            <span className="inline-flex size-8 shrink-0 items-center justify-center rounded-full bg-ink text-xs font-semibold text-surface">
              {initials}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-medium text-ink">{me.user.name}</span>
              <span className="block truncate text-xs text-ink-3">{roleLabel[me.role]}</span>
            </span>
            <Icon.More size={16} className="text-ink-3" />
          </button>
        )}
      >
        {(close) => (
          <div className="w-60">
            <div className="px-2.5 pt-1.5 pb-2 text-xs text-ink-3">{me.user.email}</div>
            <div className="px-2.5 pb-1 text-xs text-ink-3">Appearance</div>
            {themes.map((t) => (
              <MenuItem
                key={t.value}
                icon={t.icon}
                onSelect={() => {
                  setTheme(t.value);
                  close();
                }}
              >
                <span className="flex flex-1 items-center justify-between">
                  {t.label}
                  {theme === t.value && <Icon.Check size={14} className="text-ink" />}
                </span>
              </MenuItem>
            ))}
            <div className="my-1.5 border-t border-line" />
            <MenuItem
              icon={<Icon.Settings size={15} />}
              onSelect={() => {
                close();
                router.push("/settings");
              }}
            >
              Profile and settings
            </MenuItem>
            <MenuItem icon={<Icon.Logout size={15} />} onSelect={() => void signOut()}>
              Sign out
            </MenuItem>
          </div>
        )}
      </Menu>
    </div>
  );
}

function MobileBar({ onMenu, onCommand }: { onMenu: () => void; onCommand: () => void }) {
  return (
    <div className="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-line bg-canvas/90 px-4 backdrop-blur lg:hidden">
      <button
        type="button"
        onClick={onMenu}
        className="-ml-2 inline-flex size-9 items-center justify-center rounded-[10px] text-ink hover:bg-hover"
        aria-label="Open navigation"
      >
        <Icon.Menu size={18} />
      </button>
      <Link href="/" className="flex items-center gap-2 text-ink">
        <VetoMark size={20} />
        <span className="font-semibold">Veto</span>
      </Link>
      <button
        type="button"
        onClick={onCommand}
        className="-mr-2 inline-flex size-9 items-center justify-center rounded-[10px] text-ink hover:bg-hover"
        aria-label="Go to"
      >
        <Icon.Search size={18} />
      </button>
    </div>
  );
}

/** Puts the number of held actions in the tab title, so it is visible from other tabs. */
function TitleCount() {
  const queue = useQueue();
  const n = queue.data?.items.length ?? 0;
  useEffect(() => {
    const strip = (t: string) => t.replace(/^\(\d+\+?\) /, "");
    const apply = () => {
      const base = strip(document.title);
      document.title = n > 0 ? `(${n > 99 ? "99+" : n}) ${base}` : base;
    };
    apply();
    const obs = new MutationObserver(() => {
      if (n > 0 && !document.title.startsWith(`(`)) apply();
    });
    const el = document.querySelector("title");
    if (el) obs.observe(el, { childList: true });
    return () => obs.disconnect();
  }, [n]);
  return null;
}
