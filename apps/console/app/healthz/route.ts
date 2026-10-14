/** Liveness of the console process itself, for container health checks. */
export function GET() {
  return Response.json({ status: "ok" });
}
