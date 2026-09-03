# syntax=docker/dockerfile:1

# initagent marketing site — a static Vite build served by nginx. The Caddy
# edge in initagent-ops reverse-proxies `site:80` for https://initagent.dev.
# This app is never compiled into the hub binary (site/README.md).
#
# The build context is the repository root, not site/: the Vite build stages
# the hub installers from scripts/, which owns them. Keep the paths identical
# to the checkout so the relative copy in vite.config.ts resolves the same
# here as it does in a working tree.

# ── Stage 1: build ─────────────────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /src/site
COPY site/package.json site/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY scripts/install.sh scripts/install.ps1 scripts/install-macos.sh /src/scripts/
COPY site/ ./
RUN npm run build

# ── Stage 2: serve ─────────────────────────────────────────────────────
FROM nginx:1.27-alpine
COPY --from=build /src/site/dist /usr/share/nginx/html
EXPOSE 80
