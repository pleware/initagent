import { ArrowRight } from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import { HUB } from "../lib/site";
import { ROUTES } from "../lib/routes";
import { withLangParam } from "../../../web/locale.ts";
import { Screenshot } from "./Screenshot";

export function Hero() {
  const { t, i18n } = useTranslation();
  const hub = withLangParam(HUB, i18n.resolvedLanguage || i18n.language);

  return (
    <section id="top" className="relative overflow-hidden pt-24 pb-20">
      {/* Structure, not decoration: the grid matches the page's 80px rhythm and
          fades out before it reaches the content. */}
      <div
        aria-hidden
        className="grid-field pointer-events-none absolute inset-0 opacity-60 [mask-image:radial-gradient(ellipse_75%_55%_at_50%_0%,black,transparent)]"
      />
      <div
        aria-hidden
        className="pointer-events-none absolute -top-40 left-1/2 h-[420px] w-[820px] -translate-x-1/2 rounded-full bg-accent/[0.07] blur-[120px]"
      />

      <div className="relative mx-auto grid max-w-[1240px] grid-cols-1 items-center gap-14 px-5 lg:grid-cols-12 lg:gap-10 lg:px-8">
        <div className="lg:col-span-6">
          <h1 className="hero-rise text-[2.5rem] leading-[1.06] font-semibold tracking-tight sm:text-[2.9rem] lg:text-[3.1rem]">
            {t("hero.titleLine")}
            <br />
            <span className="text-accent">{t("hero.titleAccent")}</span>
          </h1>

          <p
            className="hero-rise mt-6 max-w-[46ch] text-[16.5px] leading-relaxed text-fg-muted"
            style={{ animationDelay: "0.09s" }}
          >
            {t("hero.body")}
          </p>

          <div
            className="hero-rise mt-9 flex flex-wrap items-center gap-3"
            style={{ animationDelay: "0.18s" }}
          >
            <a
              href={hub}
              className="group flex items-center gap-2 rounded-control bg-accent px-5 py-3 text-[14.5px] font-semibold whitespace-nowrap text-accent-on transition-all duration-150 hover:bg-accent-hover active:scale-[0.98]"
            >
              {t("hero.openApp")}
              <ArrowRight
                size={17}
                weight="bold"
                className="transition-transform duration-200 group-hover:translate-x-0.5"
              />
            </a>
            <a
              href={ROUTES.developers}
              className="flex items-center gap-2 rounded-control border border-line-2 bg-sidebar px-5 py-3 text-[14.5px] font-medium whitespace-nowrap text-fg transition-colors duration-150 hover:border-fg-subtle hover:bg-shell active:scale-[0.98]"
            >
              {t("hero.selfHost")}
            </a>
          </div>
          <p
            className="hero-rise mt-3 text-[13px] text-fg-subtle"
            style={{ animationDelay: "0.22s" }}
          >
            {t("hero.note")}
          </p>
        </div>

        <div
          className="hero-rise lg:col-span-6"
          style={{ animationDelay: "0.24s" }}
        >
          <figure className="relative">
            <div
              aria-hidden
              className="absolute -inset-6 rounded-[24px] bg-gradient-to-br from-accent/12 via-transparent to-transparent blur-2xl"
            />
            <Screenshot
              src="/shots/dashboard.png"
              alt={t("hero.shotAlt")}
              width={1440}
              height={900}
              eager
              className="relative w-full rounded-panel border border-line-2 shadow-[0_30px_80px_-20px_rgba(0,0,0,0.8)]"
            />
          </figure>
        </div>
      </div>
    </section>
  );
}
