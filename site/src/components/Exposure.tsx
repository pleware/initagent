import { Warning } from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import { Reveal } from "../lib/reveal";

export function Exposure() {
  const { t } = useTranslation();

  const routes = [
    {
      title: t("exposure.routes.letsEncrypt.title"),
      body: t("exposure.routes.letsEncrypt.body"),
      code: "initagent serve --tls-domain initagent.example.com --tls-email you@example.com",
    },
    {
      title: t("exposure.routes.tailscale.title"),
      body: t("exposure.routes.tailscale.body"),
      code: null,
    },
    {
      title: t("exposure.routes.proxy.title"),
      body: t("exposure.routes.proxy.body"),
      code: null,
    },
  ];

  return (
    <section className="border-t border-line-1 py-24 lg:py-32">
      <div className="mx-auto max-w-[1240px] px-5 lg:px-8">
        <Reveal>
          <div className="rounded-panel border border-accent/30 bg-accent/10 p-6 lg:p-8">
            <div className="flex items-start gap-3.5">
              <Warning
                size={22}
                weight="regular"
                className="mt-0.5 shrink-0 text-accent"
              />
              <div>
                <h2 className="text-[1.6rem] leading-[1.15] font-semibold tracking-tight sm:text-[1.9rem]">
                  {t("exposure.warningTitle")}
                </h2>
                <p className="mt-4 max-w-[70ch] text-[15.5px] leading-relaxed text-fg-muted">
                  {t("exposure.warningBeforeCode")}{" "}
                  <code className="font-mono text-fg">0.0.0.0:4200</code>{" "}
                  {t("exposure.warningAfterCode")}
                </p>
              </div>
            </div>
          </div>
        </Reveal>

        <Reveal delay={0.08}>
          <div className="mt-6 divide-y divide-line-2 overflow-hidden rounded-panel border border-line-2 bg-sidebar">
            {routes.map((r) => (
              <div
                key={r.title}
                className="grid grid-cols-1 gap-4 p-6 sm:grid-cols-12 sm:items-baseline sm:gap-8"
              >
                <h3 className="text-[15px] font-semibold tracking-tight sm:col-span-3">
                  {r.title}
                </h3>
                <div className="sm:col-span-9">
                  <p className="text-[14.5px] leading-relaxed text-fg-muted">
                    {r.body}
                  </p>
                  {r.code && (
                    <div className="mt-3 overflow-x-auto rounded-control border border-line-2 bg-canvas/60 px-4 py-3">
                      <code className="font-mono text-[12px] whitespace-pre text-fg-muted">
                        {r.code}
                      </code>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        </Reveal>
      </div>
    </section>
  );
}
