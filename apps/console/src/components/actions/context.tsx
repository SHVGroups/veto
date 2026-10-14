"use client";

import Link from "next/link";
import type { Route } from "next";
import { formatTimestamp } from "@/lib/format";
import type { ActionDetail } from "@/lib/types";
import { Icon } from "../icons";
import { RiskTag } from "../ui/pills";

const sourceLabel = { api: "Agent API", mcp: "MCP proxy", connection: "Connection" } as const;

/** Who asked for what: the request, the people and systems involved, and any untrusted text the agent read. */
export function ActionContext({ detail }: { detail: ActionDetail }) {
  const a = detail.action;
  return (
    <div className="space-y-5">
      {a.request ? (
        <blockquote className="border-l-2 border-line-strong pl-4 text-base text-ink">&ldquo;{a.request}&rdquo;</blockquote>
      ) : (
        <p className="text-sm text-ink-3">
          The agent did not say what it was asked to do. Requests that include the instruction get better decisions.
        </p>
      )}
      <dl className="grid grid-cols-2 gap-x-6 gap-y-3 text-sm sm:grid-cols-4">
        <div>
          <dt className="text-xs text-ink-3">Agent</dt>
          <dd className="mt-0.5">
            <Link href={`/agents/${a.agent_id}` as Route} className="text-ink hover:underline">
              {a.agent_name}
            </Link>
          </dd>
        </div>
        <div>
          <dt className="text-xs text-ink-3">On behalf of</dt>
          <dd className="mt-0.5 truncate text-ink" title={a.principal.id}>
            {a.principal.display || a.principal.id || "Not given"}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-ink-3">Tool</dt>
          <dd className="mt-0.5 flex flex-wrap items-center gap-2">
            <Link href={`/tools/${a.tool}` as Route} className="truncate text-ink hover:underline">
              {a.tool}
            </Link>
            {detail.tool && <RiskTag risk={detail.tool.risk_class} />}
          </dd>
        </div>
        <div>
          <dt className="text-xs text-ink-3">Received</dt>
          <dd className="mt-0.5 text-ink">
            {formatTimestamp(a.received_at)}
            <span className="block text-xs text-ink-3">via {sourceLabel[a.source]}</span>
          </dd>
        </div>
      </dl>
      {a.untrusted.length > 0 && (
        <div className="rounded-[14px] bg-safer-wash p-4">
          <div className="flex items-center gap-2 text-sm font-medium text-ink">
            <Icon.Alert size={16} className="text-safer" />
            Untrusted content the agent read before this call
          </div>
          <p className="mt-1 text-xs text-ink-2">
            Text from emails, web pages or documents. Veto shows it to the decision model as data and checks whether the call follows
            instructions hidden in it.
          </p>
          <ul className="mt-3 space-y-2">
            {a.untrusted.map((u, i) => (
              <li key={i} className="rounded-[10px] bg-surface p-3 text-sm">
                <div className="text-xs text-ink-3">{u.source}</div>
                <p className="mt-1 whitespace-pre-wrap break-words text-ink">{u.content}</p>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
