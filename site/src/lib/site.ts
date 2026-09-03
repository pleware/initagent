export const REPO = "https://github.com/pleware/initagent";
export const RELEASES = `${REPO}/releases`;
export const DOCS = `${REPO}#readme`;

/** Public marketing site and hub-installer origin. Same host serves
 *  `/install.sh`, `/install.ps1`, and `/install-macos.sh`. */
export const SITE = "https://initagent.dev";

/**
 * Where the initagent Code waitlist form posts. Left unset by default so the
 * form tells the truth instead of faking a success state. Set it in .env:
 *   VITE_WAITLIST_ENDPOINT=https://your-endpoint
 */
export const WAITLIST_ENDPOINT = import.meta.env.VITE_WAITLIST_ENDPOINT as
  | string
  | undefined;
