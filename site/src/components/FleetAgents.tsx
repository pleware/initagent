import { useTranslation } from "react-i18next";
import { Reveal } from "../lib/reveal";
import { HUB_HTTP_PORT } from "../../../web/ports.ts";

type ToolGroup = {
  id: string;
  labelKey: string;
  tools: string[];
};

const GROUPS: ToolGroup[] = [
  {
    id: "connectors",
    labelKey: "fleet.groups.connectors",
    tools: ["list_connectors", "run_command"],
  },
  {
    id: "sessions",
    labelKey: "fleet.groups.sessions",
    tools: [
      "list_sessions",
      "create_session",
      "send_input",
      "read_output",
      "kill_session",
    ],
  },
  {
    id: "files",
    labelKey: "fleet.groups.files",
    tools: ["list_files", "read_file", "write_file"],
  },
];

export function FleetAgents() {
  const { t } = useTranslation();

  return (
    <section id="agents" className="border-t border-line-1 py-24 lg:py-32">
      <div className="mx-auto grid max-w-[1240px] grid-cols-1 gap-14 px-5 lg:grid-cols-12 lg:gap-16 lg:px-8">
        <div className="lg:col-span-5">
          <Reveal>
            <h2 className="text-[2rem] leading-[1.1] font-semibold tracking-tight text-balance sm:text-[2.4rem]">
              {t("fleet.title")}
            </h2>
            <p className="mt-5 max-w-[48ch] text-[16px] leading-relaxed text-fg-muted">
              {t("fleet.body")}
            </p>
          </Reveal>

          <Reveal delay={0.08}>
            <div className="mt-8 overflow-hidden rounded-panel border border-line-2 bg-sidebar">
              <div className="overflow-x-auto p-5 font-mono text-[12.5px] leading-[1.9] whitespace-pre text-fg-muted">
                <div>
                  <span className="mr-2 text-accent select-none">$</span>
                  initagent fleet login --hub http://YOUR-HUB:{HUB_HTTP_PORT} --token TOKEN
                </div>
                <div>
                  <span className="mr-2 text-accent select-none">$</span>
                  claude mcp add initagent -- initagent mcp
                </div>
              </div>
            </div>
          </Reveal>

          <Reveal delay={0.14}>
            <figure className="mt-8 border-l-2 border-accent/60 pl-5">
              <blockquote className="text-[16px] leading-relaxed text-fg italic">
                {t("fleet.quote")}
              </blockquote>
              <figcaption className="mt-2.5 text-[13.5px] text-fg-subtle">
                {t("fleet.quoteCaption")}
              </figcaption>
            </figure>
          </Reveal>
        </div>

        <div className="lg:col-span-7">
          <Reveal delay={0.06}>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {GROUPS.map((g) => (
                <div
                  key={g.id}
                  className={`rounded-panel border border-line-2 bg-sidebar p-5 ${
                    // Sessions carries the most tools, so it takes the tall
                    // right-hand cell and the other two stack beside it.
                    g.id === "sessions"
                      ? "sm:col-start-2 sm:row-span-2 sm:row-start-1"
                      : ""
                  }`}
                >
                  <h3 className="text-[14.5px] font-semibold tracking-tight">
                    {t(g.labelKey)}
                  </h3>
                  <ul className="mt-4 flex flex-wrap gap-2">
                    {g.tools.map((tool) => (
                      <li
                        key={tool}
                        className="rounded-control border border-line-2 bg-shell px-2.5 py-1.5 font-mono text-[12px] text-fg-muted"
                      >
                        {tool}
                      </li>
                    ))}
                  </ul>
                </div>
              ))}

              <div className="rounded-panel border border-accent/25 bg-accent/10 p-5 sm:col-span-2">
                <h3 className="text-[14.5px] font-semibold tracking-tight">
                  {t("fleet.cliTitle")}
                </h3>
                <div className="mt-4 overflow-x-auto font-mono text-[12px] leading-[2] whitespace-pre text-fg-muted">
                  <div>initagent fleet connectors</div>
                  <div>initagent fleet run homelab -- git status</div>
                  <div>initagent fleet read homelab build</div>
                </div>
              </div>
            </div>
          </Reveal>
        </div>
      </div>
    </section>
  );
}
