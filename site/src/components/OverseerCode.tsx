import { useState, type FormEvent } from "react";
import { ArrowRight, BellRinging, CircleNotch, Check } from "@phosphor-icons/react";
import { useTranslation } from "react-i18next";
import { Reveal } from "../lib/reveal";
import { RELEASES, WAITLIST_ENDPOINT } from "../lib/site";

type State = "idle" | "invalid" | "sending" | "done" | "error";

function Waitlist() {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [state, setState] = useState<State>("idle");

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      setState("invalid");
      return;
    }
    setState("sending");
    try {
      const res = await fetch(WAITLIST_ENDPOINT as string, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email }),
      });
      setState(res.ok ? "done" : "error");
    } catch {
      setState("error");
    }
  }

  if (state === "done") {
    return (
      <div className="flex items-center gap-2.5 rounded-control border border-accent/35 bg-accent/10 px-4 py-3.5 text-[14.5px] text-fg">
        <Check size={17} weight="bold" className="text-accent" />
        {t("code.waitlist.done")}
      </div>
    );
  }

  return (
    <form onSubmit={submit} noValidate className="max-w-[420px]">
      <label
        htmlFor="waitlist-email"
        className="block text-[13.5px] font-medium text-fg"
      >
        {t("code.waitlist.email")}
      </label>
      <div className="mt-2 flex flex-col gap-2.5 sm:flex-row">
        <input
          id="waitlist-email"
          type="email"
          value={email}
          onChange={(e) => {
            setEmail(e.target.value);
            if (state !== "sending") setState("idle");
          }}
          placeholder={t("code.waitlist.placeholder")}
          aria-invalid={state === "invalid"}
          aria-describedby="waitlist-help"
          className="flex-1 rounded-control border border-line-2 bg-canvas px-3.5 py-2.5 text-[14.5px] text-fg placeholder:text-fg-subtle focus:border-accent focus:outline-none"
        />
        <button
          type="submit"
          disabled={state === "sending"}
          className="flex items-center justify-center gap-2 rounded-control bg-accent px-5 py-2.5 text-[14.5px] font-semibold whitespace-nowrap text-accent-on transition-all duration-150 hover:bg-accent-hover active:scale-[0.98] disabled:opacity-70"
        >
          {state === "sending" ? (
            <>
              <CircleNotch size={16} weight="bold" className="animate-spin" />
              {t("code.waitlist.sending")}
            </>
          ) : (
            t("code.waitlist.notify")
          )}
        </button>
      </div>
      <p
        id="waitlist-help"
        className={`mt-2 text-[13px] ${
          state === "invalid" || state === "error"
            ? "text-accent"
            : "text-fg-subtle"
        }`}
      >
        {state === "invalid"
          ? t("code.waitlist.invalid")
          : state === "error"
            ? t("code.waitlist.error")
            : t("code.waitlist.quiet")}
      </p>
    </form>
  );
}

export function OverseerCode() {
  const { t } = useTranslation();
  const intent = [
    {
      title: t("code.intent.startTitle"),
      body: t("code.intent.startBody"),
    },
    {
      title: t("code.intent.walkTitle"),
      body: t("code.intent.walkBody"),
    },
    {
      title: t("code.intent.installTitle"),
      body: t("code.intent.installBody"),
    },
  ];

  return (
    <section
      id="code"
      className="relative overflow-hidden border-t border-line-1 py-24 lg:py-32"
    >
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-0 bottom-0 h-[420px] bg-[radial-gradient(ellipse_60%_100%_at_50%_100%,color-mix(in_oklch,var(--ia-accent)_12%,transparent),transparent)]"
      />

      <div className="relative mx-auto max-w-[1240px] px-5 lg:px-8">
        <Reveal>
          {/* Status sits beside the title, not stacked above it as a label. */}
          <div className="flex flex-wrap items-center gap-x-5 gap-y-3">
            <h2 className="text-[2.6rem] leading-[1.03] font-semibold tracking-tight sm:text-[3.2rem] lg:text-[3.8rem]">
              {t("code.title")}
            </h2>
            <span className="rounded-control border border-accent/40 px-2.5 py-1 font-mono text-[11.5px] text-accent">
              {t("code.badge")}
            </span>
          </div>
          <p className="mt-5 max-w-[54ch] text-[17px] leading-relaxed text-fg-muted">
            {t("code.intro")}
          </p>
        </Reveal>

        <Reveal delay={0.08}>
          <div className="mt-14 grid grid-cols-1 gap-px overflow-hidden rounded-panel border border-line-2 bg-line-2 lg:grid-cols-3">
            {intent.map((item) => (
              <div key={item.title} className="bg-sidebar p-6 lg:p-7">
                <h3 className="text-[15.5px] font-semibold tracking-tight text-balance">
                  {item.title}
                </h3>
                <p className="mt-3 text-[14.5px] leading-relaxed text-fg-muted">
                  {item.body}
                </p>
              </div>
            ))}
          </div>
        </Reveal>

        <Reveal delay={0.14}>
          <div className="mt-12">
            {WAITLIST_ENDPOINT ? (
              <Waitlist />
            ) : (
              <div className="flex flex-wrap items-center gap-4">
                <a
                  href={RELEASES}
                  target="_blank"
                  rel="noreferrer"
                  className="group flex items-center gap-2 rounded-control bg-accent px-5 py-3 text-[14.5px] font-semibold whitespace-nowrap text-accent-on transition-all duration-150 hover:bg-accent-hover active:scale-[0.98]"
                >
                  <BellRinging size={17} weight="regular" />
                  {t("code.watchReleases")}
                  <ArrowRight
                    size={16}
                    weight="bold"
                    className="transition-transform duration-200 group-hover:translate-x-0.5"
                  />
                </a>
                <p className="text-[14px] text-fg-subtle">
                  {t("code.githubHint")}
                </p>
              </div>
            )}
          </div>
        </Reveal>
      </div>
    </section>
  );
}
