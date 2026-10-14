import { NextResponse, type NextRequest } from "next/server";

/**
 * The console and the API share one origin: the browser talks only to the
 * console, and these paths are forwarded to vetod. Sharing the origin keeps
 * the session cookie first-party and lets the API's CSRF checks rely on the
 * Origin header. VETO_API_URL is read per request, so one image works in any
 * environment; in development it defaults to vetod's portless address.
 */
/** Where vetod runs under portless in local development; see docs/getting-started/local-development.md. */
const DEV_API_URL = "https://api.veto.localhost";

export function proxy(request: NextRequest) {
  const upstream = process.env.VETO_API_URL || (process.env.NODE_ENV === "development" ? DEV_API_URL : undefined);
  if (!upstream) {
    return NextResponse.json(
      { error: { code: "unavailable", message: "The console is not connected to an API: set VETO_API_URL." } },
      { status: 503 },
    );
  }
  const target = new URL(request.nextUrl.pathname + request.nextUrl.search, upstream);
  return NextResponse.rewrite(target);
}

export const config = {
  matcher: ["/v1/:path*", "/mcp/:path*"],
};
