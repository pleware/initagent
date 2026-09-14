# syntax=docker/dockerfile:1

# initagent marketing site — a static Vite build served by nginx. The Caddy
# edge in initagent-ops reverse-proxies `site:80` for https://initagent.dev.
# This app is never compiled into the hub binary (site/README.md).
#
# The build context is the repository root, not site/: the Vite build stages
# the hub installers from scripts/, and the page imports shared chrome from
# web/ plus palettes from internal/brand/themes. Keep those paths identical
# to the checkout so relative imports resolve the same here as they do in a
# working tree.

# ── Stage 1: build ─────────────────────────────────────────────────────
FROM node:22-alpine AS build
WORKDIR /src/site
COPY site/package.json site/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY scripts/install.sh scripts/install.ps1 scripts/install-macos.sh /src/scripts/
COPY site/ ./
COPY web /src/web
COPY internal/brand/themes /src/internal/brand/themes
# Brand marks come from the private pware-workspace repo as a named build
# context `vendor` (provided by docker.yml). prepare-brand.mjs stages them
# into public/brand/ at build time; without the context the site keeps its
# committed placeholder chrome.
COPY --from=vendor vendor/pware/initagent /src/vendor/pware/initagent
ENV VENDOR_DIR=/src/vendor/pware/initagent
RUN npm run build

# ── Stage 2: serve ─────────────────────────────────────────────────────
FROM nginx:1.27-alpine
COPY --from=build /src/site/dist /usr/share/nginx/html
COPY site/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
