import { copyFileSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig, type Plugin } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { themeBootPlugin } from "../web/theme/boot-plugin.mjs";

const here = dirname(fileURLToPath(import.meta.url));

/** Hub installers this origin serves. `../scripts` owns them; the copies in
 *  `public/` are generated, so a fix never has to be applied twice. */
const INSTALLERS = ["install.sh", "install.ps1", "install-macos.sh"];

function installers(): Plugin {
  return {
    name: "initagent-installers",
    buildStart() {
      const out = resolve(here, "public");
      mkdirSync(out, { recursive: true });
      for (const name of INSTALLERS) {
        const from = resolve(here, "..", "scripts", name);
        try {
          copyFileSync(from, resolve(out, name));
        } catch (cause) {
          throw new Error(`cannot stage installer ${from} into public/`, { cause });
        }
      }
    },
  };
}

export default defineConfig({
  appType: "spa",
  plugins: [themeBootPlugin(), react(), tailwindcss(), installers()],
  resolve: {
    dedupe: ["react", "react-dom"],
    alias: {
      react: resolve(here, "node_modules/react"),
      "react/jsx-runtime": resolve(here, "node_modules/react/jsx-runtime.js"),
      "@ia/web": resolve(here, "../web"),
      "@base-ui/react": resolve(here, "node_modules/@base-ui/react"),
      cn: resolve(here, "node_modules/cn"),
      "class-variance-authority": resolve(here, "node_modules/class-variance-authority"),
      "@phosphor-icons/react": resolve(here, "node_modules/@phosphor-icons/react"),
      i18next: resolve(here, "node_modules/i18next"),
      "react-i18next": resolve(here, "node_modules/react-i18next"),
      "i18next-browser-languagedetector": resolve(
        here,
        "node_modules/i18next-browser-languagedetector",
      ),
    },
  },
  optimizeDeps: {
    include: ["i18next", "react-i18next", "i18next-browser-languagedetector"],
  },
  server: {
    port: 5173,
    strictPort: true,
    // index.css imports theme tokens from ../internal/brand/themes.
    fs: { allow: [".."] },
  },
});
