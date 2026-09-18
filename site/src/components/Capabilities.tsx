import {
  ArrowsClockwise,
  Cube,
  FolderOpen,
  Terminal,
  DeviceMobile,
} from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import { Reveal } from "../lib/reveal";
import { Screenshot } from "./Screenshot";

function Cell({
  className = "",
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <div
      className={`rounded-panel border border-line-2 bg-sidebar transition-colors duration-300 hover:border-fg-subtle/40 ${className}`}
    >
      {children}
    </div>
  );
}

function Copy({
  icon: Icon,
  title,
  body,
}: {
  icon: typeof Terminal;
  title: string;
  body: string;
}) {
  return (
    <div className="p-6 lg:p-7">
      <Icon size={20} weight="regular" className="text-accent" />
      <h3 className="mt-4 text-[17px] font-semibold tracking-tight">{title}</h3>
      <p className="mt-2.5 text-[14.5px] leading-relaxed text-fg-muted">{body}</p>
    </div>
  );
}

function Shot({ src, alt }: { src: string; alt: string }) {
  return (
    <Screenshot
      src={src}
      alt={alt}
      className="h-full w-full object-cover object-left-top"
    />
  );
}

export function Capabilities() {
  const { t } = useTranslation();

  return (
    <section className="border-t border-line-1 py-24 lg:py-32">
      <div className="mx-auto max-w-[1240px] px-5 lg:px-8">
        <Reveal className="max-w-[42ch]">
          <h2 className="text-[2rem] leading-[1.1] font-semibold tracking-tight text-balance sm:text-[2.4rem]">
            {t("capabilities.heading")}
          </h2>
        </Reveal>

        <Reveal delay={0.08}>
          <div className="mt-12 grid grid-cols-1 gap-4 lg:grid-cols-6">
            <Cell className="lg:col-span-2">
              <Copy
                icon={DeviceMobile}
                title={t("capabilities.sessions.title")}
                body={t("capabilities.sessions.body")}
              />
            </Cell>

            <Cell className="overflow-hidden lg:col-span-4">
              <div className="flex items-center gap-2.5 px-6 pt-6 pb-5">
                <Terminal size={18} weight="regular" className="text-accent" />
                <h3 className="text-[15.5px] font-semibold tracking-tight">
                  {t("capabilities.terminal.title")}
                </h3>
              </div>
              <div className="h-[260px] overflow-hidden border-t border-line-2">
                <Shot
                  src="/shots/terminal.png"
                  alt={t("capabilities.terminal.shotAlt")}
                />
              </div>
            </Cell>

            <Cell className="overflow-hidden lg:col-span-4">
              <div className="flex items-center gap-2.5 px-6 pt-6 pb-5">
                <FolderOpen size={18} weight="regular" className="text-accent" />
                <h3 className="text-[15.5px] font-semibold tracking-tight">
                  {t("capabilities.files.title")}
                </h3>
              </div>
              <div className="h-[260px] overflow-hidden border-t border-line-2">
                <Shot
                  src="/shots/files.png"
                  alt={t("capabilities.files.shotAlt")}
                />
              </div>
            </Cell>

            <Cell className="lg:col-span-2">
              <Copy
                icon={Cube}
                title={t("capabilities.binary.title")}
                body={t("capabilities.binary.body")}
              />
            </Cell>

            <Cell className="relative overflow-hidden lg:col-span-6">
              <div
                aria-hidden
                className="pointer-events-none absolute inset-0 bg-[radial-gradient(ellipse_60%_120%_at_88%_50%,color-mix(in_oklch,var(--ia-accent)_14%,transparent),transparent)]"
              />
              <div className="relative grid grid-cols-1 items-center gap-8 p-6 lg:grid-cols-2 lg:p-8">
                <div>
                  <ArrowsClockwise
                    size={20}
                    weight="regular"
                    className="text-accent"
                  />
                  <h3 className="mt-4 text-[17px] font-semibold tracking-tight">
                    {t("capabilities.updates.title")}
                  </h3>
                  <p className="mt-2.5 max-w-[52ch] text-[14.5px] leading-relaxed text-fg-muted">
                    {t("capabilities.updates.body")}
                  </p>
                </div>
                <div className="rounded-control border border-line-2 bg-canvas/70 p-5 font-mono text-[12.5px] leading-[2] text-fg-muted">
                  <div>
                    <span className="mr-2 text-accent select-none">$</span>
                    initagent update --check
                  </div>
                  <div>
                    <span className="mr-2 text-accent select-none">$</span>
                    initagent update
                  </div>
                  <div>
                    <span className="mr-2 text-accent select-none">$</span>
                    initagent rollback
                  </div>
                </div>
              </div>
            </Cell>
          </div>
        </Reveal>
      </div>
    </section>
  );
}
