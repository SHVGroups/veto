import type { SVGProps } from "react";

/**
 * The console's icon set, drawn for Veto on a 20-unit grid with a 1.5 stroke.
 * Icons inherit the current text colour.
 */

type IconProps = SVGProps<SVGSVGElement> & { size?: number };

function Svg({ size = 18, children, ...rest }: IconProps & { children: React.ReactNode }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
      {...rest}
    >
      {children}
    </svg>
  );
}

/** The Veto mark: three options converging on one decision. */
export function VetoMark({ size = 22, ...rest }: IconProps) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" aria-hidden="true" focusable="false" {...rest}>
      <path d="M4.5 5.5 12 19.5 19.5 5.5" stroke="currentColor" strokeWidth={2.2} strokeLinecap="round" strokeLinejoin="round" />
      <path d="M12 8.5v11" stroke="currentColor" strokeWidth={2.2} strokeLinecap="round" />
      <circle cx="4.5" cy="4.5" r="2.1" fill="currentColor" />
      <circle cx="12" cy="6.2" r="2.1" fill="currentColor" />
      <circle cx="19.5" cy="4.5" r="2.1" fill="currentColor" />
    </svg>
  );
}

export const Icon = {
  Overview: (p: IconProps) => (
    <Svg {...p}>
      <path d="M3.5 13.5a6.5 6.5 0 1 1 13 0" />
      <path d="M10 13.5 13 9" />
      <path d="M3.5 16.5h13" />
    </Svg>
  ),
  Review: (p: IconProps) => (
    <Svg {...p}>
      <path d="M3.5 11.5 5.6 4.7a1.5 1.5 0 0 1 1.4-1.2h6a1.5 1.5 0 0 1 1.4 1.2l2.1 6.8" />
      <path d="M3.5 11.5v3.5a1.5 1.5 0 0 0 1.5 1.5h10a1.5 1.5 0 0 0 1.5-1.5v-3.5h-3.8l-.9 1.8H8.2l-.9-1.8H3.5Z" />
    </Svg>
  ),
  Actions: (p: IconProps) => (
    <Svg {...p}>
      <path d="M7.5 5.5h9M7.5 10h9M7.5 14.5h9" />
      <circle cx="4" cy="5.5" r=".6" fill="currentColor" />
      <circle cx="4" cy="10" r=".6" fill="currentColor" />
      <circle cx="4" cy="14.5" r=".6" fill="currentColor" />
    </Svg>
  ),
  Audits: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="8.5" cy="8.5" r="5" />
      <path d="m12.2 12.2 4.3 4.3" />
      <path d="m6.4 8.6 1.5 1.5 2.8-3" />
    </Svg>
  ),
  Calibration: (p: IconProps) => (
    <Svg {...p}>
      <path d="M3.5 16.5h13M3.5 16.5v-13" />
      <path d="M3.5 16.5 16 4" strokeDasharray="1.5 2.2" />
      <path d="M6.5 13.2 9 11.6l2.5-2.9 2.9-2.4" />
    </Svg>
  ),
  Agents: (p: IconProps) => (
    <Svg {...p}>
      <rect x="4" y="6.5" width="12" height="9" rx="3" />
      <path d="M10 6.5V3.8" />
      <circle cx="10" cy="3.2" r=".7" fill="currentColor" />
      <path d="M7.7 10.5v1M12.3 10.5v1" />
    </Svg>
  ),
  Tools: (p: IconProps) => (
    <Svg {...p}>
      <path d="M12.6 3.7a3.5 3.5 0 0 0-4 4.8L3.9 13.2a1.6 1.6 0 0 0 2.3 2.3l4.7-4.7a3.5 3.5 0 0 0 4.8-4l-2.2 2.2-2-.4-.4-2 2.2-2.2Z" />
    </Svg>
  ),
  Policies: (p: IconProps) => (
    <Svg {...p}>
      <path d="M10 2.8 4 5v4.6c0 3.6 2.6 6.4 6 7.6 3.4-1.2 6-4 6-7.6V5l-6-2.2Z" />
      <path d="M7.4 9.8 9.2 11.6l3.5-3.6" />
    </Svg>
  ),
  Packs: (p: IconProps) => (
    <Svg {...p}>
      <path d="m10 3 7 3.6-7 3.6-7-3.6L10 3Z" />
      <path d="m3 10.2 7 3.6 7-3.6" />
      <path d="m3 13.8 7 3.6 7-3.6" />
    </Svg>
  ),
  Playground: (p: IconProps) => (
    <Svg {...p}>
      <path d="M7.5 3.5h5M8.5 3.5v4.2L4.2 15a1.3 1.3 0 0 0 1.1 2h9.4a1.3 1.3 0 0 0 1.1-2l-4.3-7.3V3.5" />
      <path d="M6 12.5h8" />
    </Svg>
  ),
  Connections: (p: IconProps) => (
    <Svg {...p}>
      <path d="M7 3.5v3M13 3.5v3" />
      <path d="M5 6.5h10v2.5a5 5 0 0 1-10 0V6.5Z" />
      <path d="M10 14v2.5" />
    </Svg>
  ),
  Mcp: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="10" cy="10" r="2.2" />
      <circle cx="4" cy="4.5" r="1.4" />
      <circle cx="16" cy="4.5" r="1.4" />
      <circle cx="10" cy="16.5" r="1.4" />
      <path d="M5.1 5.5 8.4 8.5M14.9 5.5l-3.3 3M10 12.2v2.9" />
    </Svg>
  ),
  Webhooks: (p: IconProps) => (
    <Svg {...p}>
      <path d="M8 6.5a3 3 0 1 1 4.2 2.8l2.3 4" />
      <path d="M5.6 9.6a3 3 0 1 0 3.6 4.4h4.8" />
      <circle cx="15.5" cy="14" r="1.4" />
    </Svg>
  ),
  Chat: (p: IconProps) => (
    <Svg {...p}>
      <path d="M4 5.5A2 2 0 0 1 6 3.5h8a2 2 0 0 1 2 2v6a2 2 0 0 1-2 2H9l-3.5 3v-3H6a2 2 0 0 1-2-2v-6Z" />
      <path d="M7.5 8.5h5" />
    </Svg>
  ),
  Ledger: (p: IconProps) => (
    <Svg {...p}>
      <rect x="2.8" y="7.2" width="6.6" height="5.6" rx="2.8" />
      <rect x="10.6" y="7.2" width="6.6" height="5.6" rx="2.8" />
      <path d="M8 10h4" />
    </Svg>
  ),
  Members: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="7.5" cy="7" r="2.8" />
      <path d="M2.8 16.2c.6-2.6 2.4-4 4.7-4s4.1 1.4 4.7 4" />
      <path d="M12.8 4.4a2.8 2.8 0 0 1 0 5.2M14.6 12.4c1.3.5 2.2 1.8 2.6 3.8" />
    </Svg>
  ),
  Settings: (p: IconProps) => (
    <Svg {...p}>
      <path d="M3.5 6h7M14.5 6h2M3.5 14h2M9.5 14h7" />
      <circle cx="12.5" cy="6" r="2" />
      <circle cx="7.5" cy="14" r="2" />
    </Svg>
  ),
  Status: (p: IconProps) => (
    <Svg {...p}>
      <path d="M2.5 10.5h3.2l2-5 3.6 9 2-4h4.2" />
    </Svg>
  ),
  Key: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="6.5" cy="13.5" r="3" />
      <path d="m8.6 11.4 7-7M13.2 6.8l1.8 1.8M15 5l1.6 1.6" />
    </Svg>
  ),
  Logout: (p: IconProps) => (
    <Svg {...p}>
      <path d="M11.5 3.5H6a1.5 1.5 0 0 0-1.5 1.5v10A1.5 1.5 0 0 0 6 16.5h5.5" />
      <path d="M9 10h8M14 7l3 3-3 3" />
    </Svg>
  ),
  Search: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="9" cy="9" r="5" />
      <path d="m12.8 12.8 3.7 3.7" />
    </Svg>
  ),
  ChevronRight: (p: IconProps) => (
    <Svg {...p}>
      <path d="m8 5 5 5-5 5" />
    </Svg>
  ),
  ChevronDown: (p: IconProps) => (
    <Svg {...p}>
      <path d="m5 8 5 5 5-5" />
    </Svg>
  ),
  ChevronLeft: (p: IconProps) => (
    <Svg {...p}>
      <path d="m12 5-5 5 5 5" />
    </Svg>
  ),
  Check: (p: IconProps) => (
    <Svg {...p}>
      <path d="m4.5 10.5 3.5 3.5 7.5-8" />
    </Svg>
  ),
  Close: (p: IconProps) => (
    <Svg {...p}>
      <path d="m5 5 10 10M15 5 5 15" />
    </Svg>
  ),
  Plus: (p: IconProps) => (
    <Svg {...p}>
      <path d="M10 4v12M4 10h12" />
    </Svg>
  ),
  Copy: (p: IconProps) => (
    <Svg {...p}>
      <rect x="7" y="7" width="9.5" height="9.5" rx="2" />
      <path d="M13 7V5.5A2 2 0 0 0 11 3.5H5.5a2 2 0 0 0-2 2V11a2 2 0 0 0 2 2H7" />
    </Svg>
  ),
  External: (p: IconProps) => (
    <Svg {...p}>
      <path d="M11 4h5v5M16 4l-7 7" />
      <path d="M14 11.5v3a1.5 1.5 0 0 1-1.5 1.5h-7A1.5 1.5 0 0 1 4 14.5v-7A1.5 1.5 0 0 1 5.5 6h3" />
    </Svg>
  ),
  More: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="5" cy="10" r=".8" fill="currentColor" />
      <circle cx="10" cy="10" r=".8" fill="currentColor" />
      <circle cx="15" cy="10" r=".8" fill="currentColor" />
    </Svg>
  ),
  Clock: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="10" cy="10" r="6.5" />
      <path d="M10 6.5V10l2.5 1.5" />
    </Svg>
  ),
  Alert: (p: IconProps) => (
    <Svg {...p}>
      <path d="M10 3.5 2.8 16h14.4L10 3.5Z" />
      <path d="M10 8.5v3.2" />
      <circle cx="10" cy="13.8" r=".6" fill="currentColor" />
    </Svg>
  ),
  Info: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="10" cy="10" r="6.5" />
      <path d="M10 9v4.5" />
      <circle cx="10" cy="6.6" r=".6" fill="currentColor" />
    </Svg>
  ),
  Sun: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="10" cy="10" r="3" />
      <path d="M10 2.5v1.8M10 15.7v1.8M2.5 10h1.8M15.7 10h1.8M4.7 4.7l1.3 1.3M14 14l1.3 1.3M4.7 15.3 6 14M14 6l1.3-1.3" />
    </Svg>
  ),
  Moon: (p: IconProps) => (
    <Svg {...p}>
      <path d="M15.8 12.3A6.5 6.5 0 0 1 7.7 4.2a6.5 6.5 0 1 0 8.1 8.1Z" />
    </Svg>
  ),
  Monitor: (p: IconProps) => (
    <Svg {...p}>
      <rect x="3" y="4" width="14" height="9.5" rx="1.8" />
      <path d="M7.5 16.5h5M10 13.5v3" />
    </Svg>
  ),
  Refresh: (p: IconProps) => (
    <Svg {...p}>
      <path d="M15.5 8A6 6 0 0 0 4.7 6.3M4.5 12a6 6 0 0 0 10.8 1.7" />
      <path d="M4.5 3.5v3h3M15.5 16.5v-3h-3" />
    </Svg>
  ),
  Trash: (p: IconProps) => (
    <Svg {...p}>
      <path d="M4 6h12M8 6V4.5h4V6M5.5 6l.8 9.6a1.5 1.5 0 0 0 1.5 1.4h4.4a1.5 1.5 0 0 0 1.5-1.4L14.5 6" />
    </Svg>
  ),
  Edit: (p: IconProps) => (
    <Svg {...p}>
      <path d="M12.8 4.2a1.8 1.8 0 0 1 2.6 2.6L7.5 14.7l-3.4.9.9-3.4 7.8-8Z" />
    </Svg>
  ),
  Lock: (p: IconProps) => (
    <Svg {...p}>
      <rect x="4.5" y="9" width="11" height="8" rx="2" />
      <path d="M7 9V6.5a3 3 0 0 1 6 0V9" />
    </Svg>
  ),
  Link: (p: IconProps) => (
    <Svg {...p}>
      <path d="M8.5 11.5a3 3 0 0 0 4.2 0l2.5-2.5a3 3 0 0 0-4.2-4.2l-.8.8" />
      <path d="M11.5 8.5a3 3 0 0 0-4.2 0L4.8 11a3 3 0 0 0 4.2 4.2l.8-.8" />
    </Svg>
  ),
  Download: (p: IconProps) => (
    <Svg {...p}>
      <path d="M10 3.5v9M6.5 9 10 12.5 13.5 9" />
      <path d="M4 14.5v1a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1v-1" />
    </Svg>
  ),
  Menu: (p: IconProps) => (
    <Svg {...p}>
      <path d="M3.5 6h13M3.5 10h13M3.5 14h13" />
    </Svg>
  ),
  Filter: (p: IconProps) => (
    <Svg {...p}>
      <path d="M3.5 5h13M6 10h8M8.5 15h3" />
    </Svg>
  ),
  ArrowRight: (p: IconProps) => (
    <Svg {...p}>
      <path d="M4 10h12M11.5 5.5 16 10l-4.5 4.5" />
    </Svg>
  ),
  Bolt: (p: IconProps) => (
    <Svg {...p}>
      <path d="M11 2.5 4.5 11h5l-1 6.5L15.5 9h-5l.5-6.5Z" />
    </Svg>
  ),
  Person: (p: IconProps) => (
    <Svg {...p}>
      <circle cx="10" cy="7" r="3" />
      <path d="M4.5 16.5c.7-3 2.8-4.5 5.5-4.5s4.8 1.5 5.5 4.5" />
    </Svg>
  ),
  Fingerprint: (p: IconProps) => (
    <Svg {...p}>
      <path d="M5.2 6.2A6 6 0 0 1 16 9.8v1.4" />
      <path d="M4 10a6 6 0 0 1 .3-1.8M7 16.2a12 12 0 0 1-1-5.2 4 4 0 0 1 8 0v1" />
      <path d="M10 11v1.5a9 9 0 0 0 1.4 4.4M13.8 15.4a10 10 0 0 0 .2-1.4" />
    </Svg>
  ),
};

export type IconName = keyof typeof Icon;
