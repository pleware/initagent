# initagent marketing site

The public landing page for initagent, and the announcement surface for
**initagent Code**. This is a standalone app: it is not the hub UI in
[`../ui`](../ui) and is never compiled into the `initagent` binary.

Production origin is **https://initagent.dev**. Hub installers are the same
host: `/install.sh`, `/install.ps1`, `/install-macos.sh`. There is no
separate install subdomain. Pages: home (`/#how`), **Plans** `/plans`,
**For Developers** `/developers`, **Hardware** `/hardware`. The bar leads
with **Open app** → `https://app.initagent.dev` and **Self-host** →
`/developers` (single-user, $0). We never host workers. nginx `try_files`
sends unknown paths to `index.html` so those routes work.

Those three scripts are owned by [`../scripts`](../scripts) and staged into
`public/` by the Vite build, for `npm run dev` as well as `npm run build`.
Edit them there — the copies under `public/` are generated and gitignored, so
a change made in one place reaches both the installer tests in CI and this
origin.

That is also why the container image takes the repository root as its build
context rather than this directory: installers live in `scripts/`, theme
code in `ui/src/theme`, and palettes in `internal/brand/themes`. From the
root:

```sh
docker build -f site/Dockerfile -t initagent-site .
```

React 19 + Vite 8 + Tailwind v4 + Motion. Geist / Geist Mono are self-hosted
through Fontsource, so the page makes no third-party requests at runtime.

```sh
npm install
npm run dev      # http://localhost:5173
npm run build    # -> dist/
```

## Product screenshots

Three slots read from `public/shots/`:

| File            | What to capture                                         |
| --------------- | ------------------------------------------------------- |
| `dashboard.png` | The fleet dashboard with at least one device connected  |
| `terminal.png`  | A live browser terminal attached to a session           |
| `files.png`     | The file browser listing a directory on a device        |

Until a file exists, its slot renders a labelled placeholder rather than a
broken image, so the page is presentable with none, some, or all of them
present. To capture:

```sh
../initagent serve --addr :4277 --data-dir /tmp/initagent-shots
```

Open `http://localhost:4277`, set an admin password, and screenshot at a
1440x900 viewport. The hub machine registers itself as the first device, so the
dashboard has real data immediately. Launch a session from the device page to
fill the terminal shot.

## initagent Code waitlist

The initagent Code section ships with a **Watch releases** link by default,
which is a real, working way to be notified.

If you want to collect emails instead, set an endpoint and the section swaps in
an email form with validation, sending, success, and error states:

```sh
echo 'VITE_WAITLIST_ENDPOINT=https://your-endpoint' > .env.local
```

It POSTs `{ "email": "..." }` as JSON. With no endpoint set, the form is not
rendered at all, so there is never a control that silently goes nowhere.

## Design notes

- The page is deliberately dark-only. Every screenshot on it is a dark product
  surface, and a light variant would only ever be a worse version of the page.
- One accent (`--color-beacon`, amber) across the whole page.
- Two radii, applied consistently: `rounded-panel` (10px) for surfaces,
  `rounded-control` (8px) for buttons, inputs, and tabs.
- All motion is gated behind `prefers-reduced-motion` and driven by Motion's
  viewport hooks. There are no scroll event listeners.
