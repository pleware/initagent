import { Reveal } from "../lib/reveal";
import { HARDWARE_ENQUIRE, STATIONS } from "../lib/hardware";

export function Hardware() {
  return (
    <main>
      <section className="pt-24 pb-24 lg:pt-32 lg:pb-32">
        <div className="mx-auto max-w-[1240px] px-5 lg:px-8">
          <Reveal className="max-w-[46ch]">
            <h1 className="text-[2rem] leading-[1.1] font-semibold tracking-tight text-balance sm:text-[2.4rem]">
              Your machines. Our OS.
            </h1>
            <p className="mt-5 text-[16px] leading-relaxed text-fg-muted">
              PWARE AI stations ship with PWare already installed and an
              initagent worker waiting to join a hub. The box is yours. We
              never host the workers.
            </p>
          </Reveal>

          <Reveal delay={0.08}>
            <div className="mt-14 grid grid-cols-1 gap-4 lg:grid-cols-3">
              {STATIONS.map((station) => (
                <article
                  key={station.id}
                  className={`flex flex-col rounded-panel border bg-sidebar p-6 ${
                    station.featured ? "border-accent/35" : "border-line-2"
                  }`}
                >
                  <h2 className="text-[17px] font-semibold tracking-tight">
                    {station.name}
                  </h2>
                  <p className="mt-2 text-[14.5px] text-fg-subtle">
                    {station.role}
                  </p>
                  <ul className="mt-6 flex flex-1 flex-col gap-2 text-[14.5px] leading-relaxed text-fg-muted">
                    {station.items.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                  <p className="mt-6 text-[13.5px] text-fg-subtle">
                    Price on request
                  </p>
                  <a
                    href={HARDWARE_ENQUIRE}
                    className={`mt-4 rounded-control px-4 py-2.5 text-center text-[14.5px] font-semibold transition-colors duration-150 active:scale-[0.98] ${
                      station.featured
                        ? "bg-accent text-accent-on hover:bg-accent-hover"
                        : "border border-line-2 text-fg hover:border-fg-subtle hover:bg-shell"
                    }`}
                  >
                    Enquire
                  </a>
                </article>
              ))}
            </div>
          </Reveal>
        </div>
      </section>
    </main>
  );
}
