import { Eye, GithubLogo } from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import { DOCS, RELEASES, REPO } from "../lib/site";
import { ROUTES } from "../lib/routes";

export function Footer() {
  const { t } = useTranslation();
  const links = [
    { label: t("nav.plans"), href: ROUTES.plans },
    { label: t("nav.developers"), href: ROUTES.developers },
    { label: t("nav.hardware"), href: ROUTES.hardware },
    { label: t("nav.docs"), href: DOCS },
    { label: t("nav.releases"), href: RELEASES },
    { label: t("nav.license"), href: `${REPO}/blob/main/LICENSE` },
  ];

  return (
    <footer className="border-t border-line-2 py-12">
      <div className="mx-auto flex max-w-[1240px] flex-col gap-8 px-5 sm:flex-row sm:items-center sm:justify-between lg:px-8">
        <a href={ROUTES.home} className="flex items-center gap-2.5">
          <Eye size={18} weight="regular" className="text-accent" />
          <span className="text-[14.5px] font-semibold tracking-tight">
            initagent
          </span>
          <span className="text-[13.5px] text-fg-subtle">{t("nav.licensed")}</span>
        </a>

        <nav className="flex flex-wrap items-center gap-x-6 gap-y-3">
          {links.map((l) => {
            const external = l.href.startsWith("http");
            return (
              <a
                key={l.href}
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
