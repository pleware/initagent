import { Eye, GithubLogo } from "@phosphor-icons/react";
import { DOCS, RELEASES, REPO } from "../lib/site";
import { ROUTES } from "../lib/routes";

const LINKS = [
  { label: "Plans", href: ROUTES.plans },
  { label: "For Developers", href: ROUTES.developers },
  { label: "Hardware", href: ROUTES.hardware },
  { label: "Documentation", href: DOCS },
  { label: "Releases", href: RELEASES },
  { label: "License", href: `${REPO}/blob/main/LICENSE` },
];

export function Footer() {
  return (
    <footer className="border-t border-line-2 py-12">
      <div className="mx-auto flex max-w-[1240px] flex-col gap-8 px-5 sm:flex-row sm:items-center sm:justify-between lg:px-8">
        <a href={ROUTES.home} className="flex items-center gap-2.5">
          <Eye size={18} weight="regular" className="text-accent" />
          <span className="text-[14.5px] font-semibold tracking-tight">
            initagent
          </span>
          <span className="text-[13.5px] text-fg-subtle">MIT licensed</span>
        </a>

        <nav className="flex flex-wrap items-center gap-x-6 gap-y-3">
          {LINKS.map((l) => {
            const external = l.href.startsWith("http");
            return (
              <a
                key={l.label}
                href={l.href}
                target={external ? "_blank" : undefined}
                rel={external ? "noreferrer" : undefined}
                className="text-[13.5px] text-fg-muted transition-colors hover:text-fg"
              >
                {l.label}
              </a>
            );
          })}
          <a
            href={REPO}
            target="_blank"
            rel="noreferrer"
            aria-label="initagent on GitHub"
            className="text-fg-muted transition-colors hover:text-fg"
          >
            <GithubLogo size={18} weight="regular" />
          </a>
        </nav>
      </div>
    </footer>
  );
}
