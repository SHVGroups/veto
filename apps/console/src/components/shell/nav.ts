import type { IconName } from "../icons";

export interface NavItem {
  href: string;
  label: string;
  icon: IconName;
  /** Words that find this page in the command menu. */
  keywords?: string;
  badge?: "queue" | "audits";
}

export const NAV: { group: string; items: NavItem[] }[] = [
  {
    group: "Activity",
    items: [
      { href: "/", label: "Overview", icon: "Overview", keywords: "dashboard home metrics" },
      { href: "/review", label: "Review", icon: "Review", keywords: "queue held pending approve", badge: "queue" },
      { href: "/actions", label: "Actions", icon: "Actions", keywords: "history calls log search" },
      { href: "/audits", label: "Audits", icon: "Audits", keywords: "sample verdict quality", badge: "audits" },
      { href: "/calibration", label: "Calibration", icon: "Calibration", keywords: "thresholds confidence reliability" },
    ],
  },
  {
    group: "Decisions",
    items: [
      { href: "/agents", label: "Agents", icon: "Agents", keywords: "api keys bots" },
      { href: "/tools", label: "Tools", icon: "Tools", keywords: "risk class safer rules thresholds" },
      { href: "/policies", label: "Policies", icon: "Policies", keywords: "cedar rules forbid permit" },
      { href: "/packs", label: "Question packs", icon: "Packs", keywords: "signals guards jev questions" },
      { href: "/playground", label: "Playground", icon: "Playground", keywords: "simulate test dry run" },
    ],
  },
  {
    group: "Integrations",
    items: [
      { href: "/connections", label: "Connections", icon: "Connections", keywords: "credentials http custody" },
      { href: "/mcp-servers", label: "MCP servers", icon: "Mcp", keywords: "model context protocol proxy" },
      { href: "/webhooks", label: "Webhooks", icon: "Webhooks", keywords: "events deliveries" },
      { href: "/slack", label: "Slack", icon: "Chat", keywords: "notifications approve chat" },
    ],
  },
  {
    group: "Workspace",
    items: [
      { href: "/ledger", label: "Ledger", icon: "Ledger", keywords: "audit trail hash chain verify" },
      { href: "/members", label: "Members", icon: "Members", keywords: "team invite roles" },
      { href: "/settings", label: "Settings", icon: "Settings", keywords: "workspace profile password api keys signing status" },
    ],
  },
];

export const ALL_NAV = NAV.flatMap((g) => g.items);

/** Whether a nav item is the current page or an ancestor of it. */
export function isActive(href: string, pathname: string): boolean {
  if (href === "/") return pathname === "/";
  return pathname === href || pathname.startsWith(`${href}/`);
}
