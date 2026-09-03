# syntax=docker/dockerfile:1

# initagent marketing site — a static Vite build served by nginx. The Caddy
# edge in initagent-ops reverse-proxies `site:80` for https://initagent.dev.
# This app is never compiled into the hub binary (site/README.md).

# ── Stage 1: build ─────────────────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /src
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY . .
RUN npm run build

# ── Stage 2: serve ─────────────────────────────────────────────────────
FROM nginx:1.27-alpine
COPY --from=build /src/dist /usr/share/nginx/html
EXPOSE 80
