import { useEffect, useState } from "react";
import { GithubLogo, List, X, Eye } from "@phosphor-icons/react";
import { HUB, REPO } from "../lib/site";
import { ThemeSwitcher } from "./ThemeSwitcher";

const LINKS = [
  { label: "How it works", href: "#how" },
  { label: "Fleet agents", href: "#agents" },
  { label: "initagent Code", href: "#code" },
];

export function Nav() {
  const [scrolled, setScrolled] = useState(false);
  const [open, setOpen] = useState(false);

  // IntersectionObserver on a sentinel instead of a scroll listener, so this
  // costs nothing per frame.
  useEffect(() => {
    const sentinel = document.getElementById("nav-sentinel");
    if (!sentinel) return;
    const io = new IntersectionObserver(
      ([entry]) => setScrolled(!entry.isIntersecting),
      { rootMargin: "0px" },
    );
    io.observe(sentinel);
    return () => io.disconnect();
  }, []);

  return (
    <>
      <div id="nav-sentinel" aria-hidden className="absolute top-0 h-px w-full" />
      <header
        className={`fixed inset-x-0 top-0 z-40 transition-colors duration-300 ${
          scrolled
            ? "border-b border-line-2 bg-canvas/80 backdrop-blur-xl"
            : "border-b border-transparent"
        }`}
      >
        <nav className="mx-auto flex h-16 max-w-[1240px] items-center gap-8 px-5 lg:px-8">
          <a href="#top" className="flex shrink-0 items-center gap-2.5">
            <Eye size={20} weight="regular" className="text-accent" />
            <span className="text-[15px] font-semibold tracking-tight">
              initagent
            </span>
          </a>

          <div className="hidden flex-1 items-center gap-7 md:flex">
            {LINKS.map((l) => (
              <a
                key={l.href}
                href={l.href}
                className="text-[13.5px] text-fg-muted transition-colors hover:text-fg"
              >
                {l.label}
              </a>
            ))}
          </div>

          <div className="ml-auto hidden items-center gap-3 md:flex">
            <ThemeSwitcher />
            <a
              href={REPO}
              target="_blank"
              rel="noreferrer"
              className="flex items-center gap-2 rounded-control px-3 py-2 text-[13.5px] text-fg-muted transition-colors hover:bg-shell hover:text-fg"
            >
              <GithubLogo size={17} weight="regular" />
              Source
            </a>
            <a
              href="#install"
              className="rounded-control px-3 py-2 text-[13.5px] text-fg-muted transition-colors hover:bg-shell hover:text-fg"
            >
              Self-host
            </a>
            <a
              href={HUB}
              className="rounded-control bg-accent px-4 py-2 text-[13.5px] font-semibold text-accent-on transition-transform duration-150 hover:bg-accent-hover active:scale-[0.98]"
            >
              Open app
            </a>
          </div>

          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-expanded={open}
            aria-label={open ? "Close menu" : "Open menu"}
            className="ml-auto rounded-control p-2 text-fg-muted transition-colors hover:text-fg md:hidden"
          >
            {open ? <X size={20} /> : <List size={20} />}
          </button>
        </nav>

        {open && (
          <div className="border-t border-line-2 bg-canvas/95 px-5 py-4 backdrop-blur-xl md:hidden">
            <div className="flex flex-col gap-1">
              {LINKS.map((l) => (
                <a
                  key={l.href}
                  href={l.href}
                  onClick={() => setOpen(false)}
                  className="rounded-control px-2 py-3 text-[15px] text-fg-muted hover:bg-shell hover:text-fg"
                >
                  {l.label}
                </a>
              ))}
              <ThemeSwitcher className="px-2 py-2" />
              <a
                href={REPO}
                target="_blank"
                rel="noreferrer"
                className="rounded-control px-2 py-3 text-[15px] text-fg-muted hover:bg-shell hover:text-fg"
              >
                Source
              </a>
              <a
                href="#install"
                onClick={() => setOpen(false)}
                className="rounded-control px-2 py-3 text-[15px] text-fg-muted hover:bg-shell hover:text-fg"
              >
                Self-host
              </a>
              <a
                href={HUB}
                className="mt-2 rounded-control bg-accent px-4 py-2.5 text-center text-[15px] font-semibold text-accent-on"
              >
                Open app
              </a>
            </div>
          </div>
        )}
      </header>
    </>
  );
}
