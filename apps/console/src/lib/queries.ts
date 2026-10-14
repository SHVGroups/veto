"use client";

import { keepPreviousData, useQuery, type UseQueryOptions } from "@tanstack/react-query";
import { api, toolPath } from "./api";
import type {
  ActionDetail,
  ActionPage,
  AgentDetail,
  AgentStats,
  APIKey,
  AuditItem,
  Calibration,
  Connection,
  Delivery,
  Invitation,
  Items,
  KeyView,
  LedgerPage,
  MCPServer,
  Me,
  Member,
  Meta,
  Overview,
  PackInfo,
  PackVersion,
  PolicySummary,
  PolicyVersion,
  Range,
  SetupState,
  SlackStatus,
  SystemStatus,
  Thresholds,
  Tool,
  ToolStats,
  Webhook,
  Workspace,
} from "./types";

/** Query keys, grouped so the live stream can invalidate whole families. */
export const keys = {
  setup: ["setup"] as const,
  me: ["me"] as const,
  meta: ["meta"] as const,
  overview: (range: Range) => ["overview", range] as const,
  calibration: (range: Range, tool: string) => ["calibration", range, tool] as const,
  actions: (filter: ActionFilter) => ["actions", filter] as const,
  queue: ["actions", "queue"] as const,
  action: (id: string) => ["action", id] as const,
  audits: (state: "open" | "reviewed") => ["audits", state] as const,
  agents: ["agents"] as const,
  agent: (id: string) => ["agents", id] as const,
  keys: (agent?: string) => ["keys", agent ?? ""] as const,
  tools: ["tools"] as const,
  tool: (name: string) => ["tools", name] as const,
  policies: ["policies"] as const,
  policy: (v: number) => ["policies", v] as const,
  activePolicy: ["policies", "active"] as const,
  packs: ["packs"] as const,
  packVersions: (name: string) => ["packs", name, "versions"] as const,
  connections: ["connections"] as const,
  mcpServers: ["mcp-servers"] as const,
  webhooks: ["webhooks"] as const,
  deliveries: (id: string) => ["webhooks", id, "deliveries"] as const,
  slack: ["slack"] as const,
  workspace: ["workspace"] as const,
  members: ["members"] as const,
  invitations: ["invitations"] as const,
  ledger: (kind: string, action: string, before: number) => ["ledger", kind, action, before] as const,
  status: ["status"] as const,
};

export interface ActionFilter {
  status?: string;
  agent?: string;
  tool?: string;
  decided_by?: string;
  option?: string;
  source?: string;
  q?: string;
  held?: boolean;
  since?: string;
  until?: string;
  cursor?: string;
  limit?: number;
}

type Opts<T> = Omit<UseQueryOptions<T, Error, T, readonly unknown[]>, "queryKey" | "queryFn">;

export const useSetupState = () => useQuery({ queryKey: keys.setup, queryFn: () => api.get<SetupState>("/v1/setup"), staleTime: 60_000 });
export const useMe = (opts?: Opts<Me>) =>
  useQuery({ queryKey: keys.me, queryFn: () => api.get<Me>("/v1/auth/me"), staleTime: 60_000, ...opts });
export const useMeta = () => useQuery({ queryKey: keys.meta, queryFn: () => api.get<Meta>("/v1/meta"), staleTime: 5 * 60_000 });

export const useOverview = (range: Range) =>
  useQuery({
    queryKey: keys.overview(range),
    queryFn: () => api.get<Overview>("/v1/overview", { range }),
    placeholderData: keepPreviousData,
    refetchInterval: 60_000,
  });

export const useCalibration = (range: Range, tool: string) =>
  useQuery({
    queryKey: keys.calibration(range, tool),
    queryFn: () => api.get<Calibration>("/v1/calibration", { range, tool }),
    placeholderData: keepPreviousData,
  });

export const useActions = (filter: ActionFilter) =>
  useQuery({
    queryKey: keys.actions(filter),
    queryFn: () => api.get<ActionPage>("/v1/actions", { ...filter }),
    placeholderData: keepPreviousData,
  });

/** Actions waiting for a person, oldest deadline first is applied by the caller. */
export const useQueue = () =>
  useQuery({
    queryKey: keys.queue,
    queryFn: () => api.get<ActionPage>("/v1/actions", { status: "pending_review", limit: 200 }),
    refetchInterval: 30_000,
  });

export const useActionDetail = (id: string) =>
  useQuery({ queryKey: keys.action(id), queryFn: () => api.get<ActionDetail>(`/v1/actions/${encodeURIComponent(id)}/detail`) });

export const useAudits = (state: "open" | "reviewed") =>
  useQuery({ queryKey: keys.audits(state), queryFn: () => api.get<Items<AuditItem>>("/v1/audits", { state }) });

export const useAgents = () => useQuery({ queryKey: keys.agents, queryFn: () => api.get<Items<AgentStats>>("/v1/agents") });
export const useAgent = (id: string) =>
  useQuery({ queryKey: keys.agent(id), queryFn: () => api.get<AgentDetail>(`/v1/agents/${encodeURIComponent(id)}`) });
export const useKeys = (agent?: string) =>
  useQuery({ queryKey: keys.keys(agent), queryFn: () => api.get<Items<APIKey>>("/v1/keys", { agent }) });

export const useTools = () => useQuery({ queryKey: keys.tools, queryFn: () => api.get<Items<ToolStats>>("/v1/tools") });
export const useTool = (name: string) =>
  useQuery({
    queryKey: keys.tool(name),
    queryFn: () => api.get<{ tool: Tool; effective: Thresholds }>(`/v1/tools/${toolPath(name)}`),
  });

export const usePolicies = () => useQuery({ queryKey: keys.policies, queryFn: () => api.get<Items<PolicyVersion>>("/v1/policies") });
export const useActivePolicy = () =>
  useQuery({
    queryKey: keys.activePolicy,
    queryFn: () => api.get<{ version: PolicyVersion; policies: PolicySummary[] | null }>("/v1/policies/active"),
    retry: false,
  });

export const usePacks = () => useQuery({ queryKey: keys.packs, queryFn: () => api.get<Items<PackInfo>>("/v1/packs") });
export const usePackVersions = (name: string) =>
  useQuery({
    queryKey: keys.packVersions(name),
    queryFn: () => api.get<Items<PackVersion>>(`/v1/packs/${encodeURIComponent(name)}/versions`),
  });

export const useConnections = () => useQuery({ queryKey: keys.connections, queryFn: () => api.get<Items<Connection>>("/v1/connections") });
export const useMCPServers = () => useQuery({ queryKey: keys.mcpServers, queryFn: () => api.get<Items<MCPServer>>("/v1/mcp-servers") });
export const useWebhooks = () => useQuery({ queryKey: keys.webhooks, queryFn: () => api.get<Items<Webhook>>("/v1/webhooks") });
export const useDeliveries = (id: string) =>
  useQuery({
    queryKey: keys.deliveries(id),
    queryFn: () => api.get<Items<Delivery>>(`/v1/webhooks/${encodeURIComponent(id)}/deliveries`),
    refetchInterval: 15_000,
  });
export const useSlack = () => useQuery({ queryKey: keys.slack, queryFn: () => api.get<SlackStatus>("/v1/integrations/slack") });

export const useWorkspace = () => useQuery({ queryKey: keys.workspace, queryFn: () => api.get<Workspace>("/v1/workspace") });
export const useMembers = () => useQuery({ queryKey: keys.members, queryFn: () => api.get<Items<Member>>("/v1/members") });
export const useInvitations = () => useQuery({ queryKey: keys.invitations, queryFn: () => api.get<Items<Invitation>>("/v1/invitations") });

export const useLedger = (kind: string, action: string, before: number) =>
  useQuery({
    queryKey: keys.ledger(kind, action, before),
    queryFn: () => api.get<LedgerPage>("/v1/ledger", { kind, action, before: before || undefined, limit: 50 }),
    placeholderData: keepPreviousData,
  });

export const useStatus = () => useQuery({ queryKey: keys.status, queryFn: () => api.get<SystemStatus>("/v1/status") });

export type { KeyView };
