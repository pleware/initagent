import { Install } from "./Install";
import { FleetAgents } from "./FleetAgents";
import { Exposure } from "./Exposure";
import { OverseerCode } from "./OverseerCode";
import { Reveal } from "../lib/reveal";

export function Developers() {
  return (
    <main>
      <section className="pt-24 pb-8 lg:pt-32">
        <div className="mx-auto max-w-[1240px] px-5 lg:px-8">
          <Reveal className="max-w-[46ch]">
            <h1 className="text-[2rem] leading-[1.1] font-semibold tracking-tight text-balance sm:text-[2.4rem]">
              Run the same binary on your machines.
            </h1>
            <p className="mt-5 text-[16px] leading-relaxed text-fg-muted">
              Self-host is $0. Install the hub, join a device, launch an
              agent. We never host your workers.
            </p>
          </Reveal>
        </div>
      </section>
      <Install />
      <FleetAgents />
      <Exposure />
      <OverseerCode />
    </main>
  );
}
