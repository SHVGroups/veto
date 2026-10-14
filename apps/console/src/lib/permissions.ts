import type { Role } from "./types";

/** Mirrors the server's role model. The server enforces it; the console only hides what would fail. */
export type Permission = "view" | "review" | "configure" | "manage_members" | "own";

const order: Permission[] = ["view", "review", "configure", "manage_members", "own"];

const ceiling: Record<Role, Permission> = {
  owner: "own",
  admin: "manage_members",
  reviewer: "review",
  viewer: "view",
};

export function can(role: Role | undefined, p: Permission): boolean {
  if (!role) return false;
  return order.indexOf(p) <= order.indexOf(ceiling[role]);
}
