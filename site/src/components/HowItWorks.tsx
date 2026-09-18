import { Browsers, HardDrives, Cloud, Laptop } from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import { Reveal } from "../lib/reveal";

function Node({
  icon: Icon,
  title,
  sub,
  accent = false,
}: {
  icon: typeof Browsers;
  title: string;
  sub: string;
  accent?: boolean;
}) {
  return (
    <div
      className={`rounded-panel border px-5 py-4 text-center ${
        accent
          ? "border-accent/35 bg-accent/10"
          : "border-line-2 bg-sidebar"
      }`}
    >
      <Icon
        size={20}
        weight="regular"
        className={`mx-auto ${accent ? "text-accent" : "text-fg-muted"}`}
      />
      <div className="mt-2.5 text-[14.5px] font-semibold tracking-tight">
        {title}
      </div>
      <div className="mt-1 font-mono text-[11.5px] text-fg-subtle">{sub}</div>
    </div>
  );
}

export function HowItWorks() {
  const { t } = useTranslation();
  const agents = [
    { icon: Laptop, title: t("how.nodeMacbook"), sub: t("how.launchd") },
    { icon: HardDrives, title: t("how.nodeHomelab"), sub: t("how.systemd") },
    { icon: Cloud, title: t("how.nodeCloud"), sub: t("how.systemd") },
  ];

  return (
    <section id="how" className="relative border-t border-line-1 py-24 lg:py-32">
      <div
        aria-hidden
        className="grid-field pointer-events-none absolute inset-0 opacity-40 [mask-image:radial-gradient(ellipse_60%_60%_at_50%_50%,black,transparent)]"
      />

      <div className="relative mx-auto max-w-[1240px] px-5 lg:px-8">
        <Reveal className="max-w-[46ch]">
          <h2 className="text-[2rem] leading-[1.1] font-semibold tracking-tight text-balance sm:text-[2.4rem]">
            {t("how.heading")}
          </h2>
          <p className="mt-5 text-[16px] leading-relaxed text-fg-muted">
            {t("how.body")}
          </p>
        </Reveal>

        <Reveal delay={0.1}>
          <div className="mx-auto mt-16 max-w-[760px]">
            <div className="mx-auto max-w-[280px]">
              <Node
                icon={Browsers}
                title={t("how.browser")}
                sub={t("how.browserSub")}
              />
            </div>

            <div className="relative flex justify-center py-3">
              <div className="h-12 w-px bg-line-2" />
              <span className="absolute top-1/2 left-1/2 ml-3 -translate-y-1/2 font-mono text-[11px] whitespace-nowrap text-fg-subtle">
                HTTPS + WebSocket
              </span>
            </div>

            <div className="mx-auto max-w-[280px]">
              <Node
                icon={HardDrives}
                title={t("how.hub")}
                sub={t("how.hubSub")}
                accent
              />
            </div>

            <div className="relative flex justify-center py-3">
              <div className="h-12 w-px bg-line-2" />
              <span className="absolute top-1/2 left-1/2 ml-3 -translate-y-1/2 font-mono text-[11px] whitespace-nowrap text-fg-subtle">
                {t("how.outbound")}
              </span>
            </div>

            {/* Fan-out rail. Hidden below sm, where the nodes stack instead. */}
            <div className="mx-auto hidden h-px w-2/3 bg-line-2 sm:block" />

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3 sm:gap-5">
              {agents.map((a) => (
                <div key={a.title}>
                  <div className="mx-auto hidden h-8 w-px bg-line-2 sm:block" />
                  <div className="mx-auto flex h-6 w-px justify-center bg-line-2 sm:hidden" />
                  <Node icon={a.icon} title={a.title} sub={a.sub} />
                </div>
              ))}
            </div>

            <p className="mt-8 text-center text-[13.5px] text-fg-subtle">
              {t("how.agentsFootnote")}
            </p>
          </div>
        </Reveal>
      </div>
    </section>
  );
}
