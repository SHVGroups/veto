"use client";

import type { Route } from "next";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { cx } from "@/lib/cx";
import { useAgents, useTools } from "@/lib/queries";
import { Icon } from "../icons";
import { Kbd } from "../ui/layout";
import { ALL_NAV } from "./nav";

interface Entry {
  id: string;
  label: string;
  hint: string;
  href: string;
  icon: ReactNode;
  haystack: string;
}

const ID_PATTERN = /^(act|agt|aud|key|con|mcp|whk)_[0-9a-z]{20,}$/;

/** Jump anywhere: pages, agents, tools, or an id pasted from a log or webhook. */
export function CommandMenu({ open, onClose }: { open: boolean; onClose: () => void }) {
  const router = useRouter();
  const ref = useRef<HTMLDialogElement>(null);
  const input = useRef<HTMLInputElement>(null);
  const [q, setQ] = useState("");
  const [sel, setSel] = useState(0);
  const tools = useTools();
  const agents = useAgents();

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) {
      setQ("");
      setSel(0);
      d.showModal();
      window.setTimeout(() => input.current?.focus(), 0);
    }
    if (!open && d.open) d.close();
  }, [open]);

  const entries = useMemo<Entry[]>(() => {
    const out: Entry[] = ALL_NAV.map((n) => {
      const I = Icon[n.icon];
      return {
        id: `nav:${n.href}`,
        label: n.label,
        hint: "Page",
        href: n.href,
        icon: <I size={16} />,
        haystack: `${n.label} ${n.keywords ?? ""}`.toLowerCase(),
      };
    });
    for (const a of agents.data?.items ?? []) {
      out.push({
        id: `agent:${a.id}`,
        label: a.name,
        hint: "Agent",
        href: `/agents/${a.id}`,
        icon: <Icon.Agents size={16} />,
        haystack: `${a.name} ${a.vendor} agent`.toLowerCase(),
      });
    }
    for (const t of tools.data?.items ?? []) {
      out.push({
        id: `tool:${t.name}`,
        label: t.name,
        hint: "Tool",
        href: `/tools/${t.name}`,
        icon: <Icon.Tools size={16} />,
        haystack: `${t.name} ${t.description} tool`.toLowerCase(),
      });
    }
    return out;
  }, [agents.data, tools.data]);

  const results = useMemo(() => {
    const term = q.trim().toLowerCase();
    const list: Entry[] = [];
    if (ID_PATTERN.test(term)) {
      const prefix = term.slice(0, term.indexOf("_"));
      const href = prefix === "act" ? `/actions/${term}` : prefix === "agt" ? `/agents/${term}` : null;
      if (href)
        list.push({ id: `id:${term}`, label: `Open ${term}`, hint: "Id", href, icon: <Icon.ArrowRight size={16} />, haystack: term });
    }
    if (!term) return [...list, ...entries.filter((e) => e.id.startsWith("nav:"))];
    const words = term.split(/\s+/);
    return [...list, ...entries.filter((e) => words.every((w) => e.haystack.includes(w)))].slice(0, 30);
  }, [q, entries]);

  function go(e: Entry | undefined) {
    if (!e) return;
    onClose();
    router.push(e.href as Route);
  }

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onClick={(e) => e.target === ref.current && onClose()}
      className="m-auto mt-[12vh] w-[calc(100%-2rem)] max-w-xl overflow-hidden rounded-[18px] bg-surface p-0 text-ink shadow-pop backdrop:bg-[rgb(10_11_13/0.35)] backdrop:backdrop-blur-[2px]"
    >
      <div className="flex items-center gap-3 border-b border-line px-4">
        <Icon.Search size={17} className="text-ink-3" />
        <input
          ref={input}
          value={q}
          onChange={(e) => {
            setQ(e.target.value);
            setSel(0);
          }}
          onKeyDown={(e) => {
            if (e.key === "ArrowDown") {
              e.preventDefault();
              setSel((s) => Math.min(results.length - 1, s + 1));
            } else if (e.key === "ArrowUp") {
              e.preventDefault();
              setSel((s) => Math.max(0, s - 1));
            } else if (e.key === "Enter") {
              e.preventDefault();
              go(results[sel]);
            }
          }}
          placeholder="Go to a page, agent or tool, or paste an action id"
          className="h-13 flex-1 bg-transparent text-base text-ink outline-none placeholder:text-ink-3"
          aria-label="Search"
        />
        <Kbd>Esc</Kbd>
      </div>
      <ul className="scroll-quiet max-h-[52vh] overflow-y-auto p-2" role="listbox">
        {results.length === 0 && <li className="px-3 py-6 text-center text-sm text-ink-3">Nothing matches &ldquo;{q}&rdquo;.</li>}
        {results.map((r, i) => (
          <li key={r.id} role="option" aria-selected={i === sel}>
            <button
              type="button"
              onMouseMove={() => setSel(i)}
              onClick={() => go(r)}
              className={cx(
                "flex w-full items-center gap-3 rounded-[10px] px-3 py-2.5 text-left text-sm",
                i === sel ? "bg-hover text-ink" : "text-ink-2",
              )}
            >
              <span className="text-ink-3">{r.icon}</span>
              <span className="flex-1 truncate">{r.label}</span>
              <span className="text-xs text-ink-3">{r.hint}</span>
            </button>
          </li>
        ))}
      </ul>
    </dialog>
  );
}
