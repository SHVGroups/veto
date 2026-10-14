# syntax=docker/dockerfile:1.7
# vetod: one static binary on a distroless base.

FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=1.0.0
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X github.com/shvgroups/veto/server/internal/app.Version=${VERSION}" \
      -o /out/vetod ./cmd/vetod

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/vetod /usr/local/bin/vetod
USER nonroot:nonroot
EXPOSE 8080
ENV VETO_LISTEN_ADDR=:8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 CMD ["/usr/local/bin/vetod", "healthcheck"]
ENTRYPOINT ["/usr/local/bin/vetod"]
CMD ["serve"]
