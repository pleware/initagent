/** Marketing paths. How it works stays an in-page anchor on home. */

export const ROUTES = {
  home: "/",
  how: "/#how",
  plans: "/plans",
  developers: "/developers",
  hardware: "/hardware",
} as const;

export type NavItem = {
  label: string;
  href: string;
};

export const NAV: readonly NavItem[] = [
  { label: "How it works", href: ROUTES.how },
  { label: "Plans", href: ROUTES.plans },
  { label: "For Developers", href: ROUTES.developers },
  { label: "Hardware", href: ROUTES.hardware },
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
