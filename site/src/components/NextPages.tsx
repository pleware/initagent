import { Reveal } from "../lib/reveal";
import { ROUTES } from "../lib/routes";

const DOORS = [
  {
    href: ROUTES.plans,
    title: "Plans",
    body: "Pay for people. We never host workers.",
  },
  {
    href: ROUTES.developers,
    title: "For Developers",
    body: "Self-host the same binary. Installers and MCP.",
  },
  {
    href: ROUTES.hardware,
    title: "Hardware",
    body: "Stations with PWare already on the disk.",
  },
];

export function NextPages() {
  return (
    <section className="border-t border-line-1 py-24 lg:py-32">
      <div className="mx-auto max-w-[1240px] px-5 lg:px-8">
        <Reveal>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            {DOORS.map((door) => (
              <a
                key={door.href}
                href={door.href}
                className="rounded-panel border border-line-2 bg-sidebar p-6 transition-colors hover:border-fg-subtle hover:bg-shell"
              >
                <h2 className="text-[17px] font-semibold tracking-tight">
                  {door.title}
                </h2>
                <p className="mt-2.5 text-[14.5px] leading-relaxed text-fg-muted">
                  {door.body}
                </p>
              </a>
            ))}
          </div>
        </Reveal>
      </div>
    </section>
  );
}
