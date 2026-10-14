# syntax=docker/dockerfile:1.7
# The console: a standalone Next.js server. Build from the repository root:
#   docker build -f apps/console/Dockerfile .

FROM node:22-alpine AS deps
RUN corepack enable
WORKDIR /repo
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/console/package.json apps/console/
COPY sdks/typescript/package.json sdks/typescript/
COPY server/package.json server/
RUN --mount=type=cache,target=/root/.local/share/pnpm/store pnpm install --frozen-lockfile --filter @veto/console...

FROM deps AS build
COPY apps/console apps/console
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm --filter @veto/console build

FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1 PORT=3000 HOSTNAME=0.0.0.0
RUN addgroup -S veto && adduser -S veto -G veto
COPY --from=build --chown=veto:veto /repo/apps/console/.next/standalone ./
COPY --from=build --chown=veto:veto /repo/apps/console/.next/static ./apps/console/.next/static
COPY --from=build --chown=veto:veto /repo/apps/console/public ./apps/console/public
USER veto
EXPOSE 3000
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3000/healthz >/dev/null || exit 1
CMD ["node", "apps/console/server.js"]
