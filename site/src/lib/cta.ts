import { withLangParam } from "../../../web/locale.ts";
import { ROUTES } from "./routes";
import { HUB, SITE } from "./site";

/** Hub bounce that counts an Open-app click, then 302s to the cockpit. */
export function openAppHref(lang: string): string {
  return withLangParam(`${HUB}/r/cta/open_app`, lang);
}

/** Hub bounce that counts a Self-host click, then 302s to Developers. */
export function selfHostHref(): string {
  const url = new URL(`${HUB}/r/cta/self_host`);
  url.searchParams.set("to", `${SITE}${ROUTES.developers}`);
  return url.toString();
}
