# syntax=docker/dockerfile:1

# initagent hub image. Multi-stage: build the cockpit UI, compile the single
# binary with that UI embedded, then copy it into a minimal runtime.
#
# One image serves both offerings (18): self-host runs it against SQLite,
# the hosted hub points it at Postgres with INITAGENT_DATABASE_URL. The
# deploy chooses with environment, not a second image.

# ── Stage 1: cockpit UI ────────────────────────────────────────────────
FROM node:22-alpine AS ui
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY ui/ ./
# The cockpit reads its colour tokens from internal/brand/themes, which sits
# outside this stage's ui/ context. Keep the path identical to the checkout so
# the relative @import in src/index.css resolves the same here as it does in a
# working tree.
COPY internal/brand/themes /src/internal/brand/themes
RUN npm run build

# ── Stage 2: Go build ──────────────────────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /src
ARG VERSION=dev
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/ui/dist ./cmd/overseer/uidist
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/initagent ./cmd/overseer

# ── Stage 3: runtime ───────────────────────────────────────────────────
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 65532 initagent
COPY --from=build /out/initagent /usr/local/bin/initagent
USER initagent
EXPOSE 4200
ENTRYPOINT ["initagent"]
CMD ["serve", "--addr", ":4200"]
