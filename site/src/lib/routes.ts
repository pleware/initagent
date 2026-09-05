/** Marketing paths. How it works stays an in-page anchor on home. */

export const ROUTES = {
  home: "/",
  how: "/#how",
  plans: "/plans",
  developers: "/developers",
  hardware: "/hardware",
} as const;

export type NavItem = {
  key: "how" | "plans" | "developers" | "hardware";
  href: string;
};

export const NAV: readonly NavItem[] = [
  { key: "how", href: ROUTES.how },
  { key: "plans", href: ROUTES.plans },
  { key: "developers", href: ROUTES.developers },
  { key: "hardware", href: ROUTES.hardware },
];

export function currentPath(): string {
  const p = window.location.pathname.replace(/\/+$/, "");
  return p === "" ? "/" : p;
}

export function navIsCurrent(href: string, path: string): boolean {
  if (href.startsWith("/#") || href.startsWith("#")) {
    return false;
  }
  return path === href;
}
