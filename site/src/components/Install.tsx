import { useState } from "react";
import { AppleLogo, Check, Copy, LinuxLogo, WindowsLogo } from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import { SITE } from "../lib/site";
import { Reveal } from "../lib/reveal";

type Platform = {
  id: string;
  label: string;
  icon: typeof LinuxLogo;
  shell: string;
  command: string;
  noteKey: string;
};

const PLATFORMS: Platform[] = [
  {
    id: "linux",
    label: "Linux",
    icon: LinuxLogo,
    shell: "sh",
    command: `curl -fsSL ${SITE}/install.sh | sh`,
    noteKey: "install.linuxNote",
  },
  {
    id: "macos",
    label: "macOS",
    icon: AppleLogo,
    shell: "sh",
    command: `curl -fsSL ${SITE}/install-macos.sh | sh`,
    noteKey: "install.macosNote",
  },
  {
    id: "windows",
    label: "Windows",
    icon: WindowsLogo,
    shell: "powershell",
    command: `irm ${SITE}/install.ps1 | iex`,
    noteKey: "install.windowsNote",
  },
];

const BEATS = [
  {
    id: "install",
    titleKey: "install.beats.install.title",
    bodyKey: "install.beats.install.body",
  },
  {
    id: "join",
    titleKey: "install.beats.join.title",
    bodyKey: "install.beats.join.body",
  },
  {
    id: "launch",
    titleKey: "install.beats.launch.title",
    bodyKey: "install.beats.launch.body",
  },
];

function CommandBlock({ platform }: { platform: Platform }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(platform.command);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1800);
    } catch {
      setCopied(false);
    }
  }

  return (
    <div className="overflow-hidden rounded-panel border border-line-2 bg-sidebar">
      <div className="flex items-center justify-between gap-4 border-b border-line-2 px-4 py-2.5">
        <span className="font-mono text-[11.5px] tracking-wide text-fg-subtle">
          {platform.shell}
        </span>
        <button
          type="button"
          onClick={copy}
          className="flex items-center gap-1.5 rounded-control px-2 py-1 font-mono text-[11.5px] text-fg-muted transition-colors hover:bg-shell hover:text-fg active:scale-[0.98]"
        >
          {copied ? (
            <>
              <Check size={13} weight="bold" className="text-accent" />
              {t("install.copied")}
            </>
          ) : (
            <>
              <Copy size={13} weight="regular" />
              {t("install.copy")}
            </>
          )}
        </button>
      </div>
      <div className="overflow-x-auto px-4 py-4 sm:px-5 sm:py-5">
        <code className="font-mono text-[12.5px] leading-relaxed whitespace-pre text-fg sm:text-[13.5px]">
          <span className="mr-2 text-accent select-none">$</span>
          {platform.command}
        </code>
      </div>
    </div>
  );
}

export function Install() {
  const { t } = useTranslation();
  const [active, setActive] = useState(PLATFORMS[0]);

  return (
    <section id="install" className="border-t border-line-1 py-24 lg:py-32">
      <div className="mx-auto max-w-[1240px] px-5 lg:px-8">
        <Reveal className="max-w-[46ch]">
          <p className="font-mono text-[12px] tracking-wide text-accent">
            {t("nav.selfHost")}
          </p>
          <h2 className="mt-3 text-[2rem] leading-[1.1] font-semibold tracking-tight text-balance sm:text-[2.4rem]">
            {t("install.title")}
          </h2>
          <p className="mt-5 text-[16px] leading-relaxed text-fg-muted">
            {t("install.body")}
          </p>
        </Reveal>

        <Reveal delay={0.08} className="mt-10">
          <div
            role="tablist"
            aria-label={t("install.tabsAriaLabel")}
            className="mb-3 flex flex-wrap gap-1.5"
          >
            {PLATFORMS.map((p) => {
              const Icon = p.icon;
              const on = p.id === active.id;
              return (
                <button
                  key={p.id}
                  role="tab"
                  aria-selected={on}
                  onClick={() => setActive(p)}
                  className={`flex items-center gap-2 rounded-control border px-3.5 py-2 text-[13.5px] transition-colors duration-150 active:scale-[0.98] ${
                    on
                      ? "border-line-2 bg-shell text-fg"
                      : "border-transparent text-fg-muted hover:bg-sidebar hover:text-fg"
                  }`}
                >
                  <Icon size={16} weight="regular" />
                  {p.label}
                </button>
              );
            })}
          </div>

          <CommandBlock platform={active} />
          <p className="mt-3 text-[13.5px] text-fg-subtle">{t(active.noteKey)}</p>
        </Reveal>

        <Reveal delay={0.14}>
          <ol className="mt-16 grid grid-cols-1 gap-px overflow-hidden rounded-panel border border-line-2 bg-line-2 sm:grid-cols-3">
            {BEATS.map((b, i) => (
              <li key={b.id} className="bg-sidebar px-6 py-7">
                <div className="flex items-baseline gap-3">
                  <span className="font-mono text-[12px] text-accent">
                    {String(i + 1).padStart(2, "0")}
                  </span>
                  <h3 className="text-[15.5px] font-semibold tracking-tight">
                    {t(b.titleKey)}
                  </h3>
                </div>
                <p className="mt-2.5 text-[14px] leading-relaxed text-fg-muted">
                  {t(b.bodyKey)}
                </p>
              </li>
            ))}
          </ol>
        </Reveal>
      </div>
    </section>
  );
}
