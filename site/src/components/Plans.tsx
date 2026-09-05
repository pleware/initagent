import { Reveal } from "../lib/reveal";
import { PLANS } from "../lib/plans";

export function Plans() {
  return (
    <main>
      <section className="pt-24 pb-24 lg:pt-32 lg:pb-32">
        <div className="mx-auto max-w-[1240px] px-5 lg:px-8">
          <Reveal className="max-w-[46ch]">
            <h1 className="text-[2rem] leading-[1.1] font-semibold tracking-tight text-balance sm:text-[2.4rem]">
              Pay for people. Machines stay yours.
            </h1>
            <p className="mt-5 text-[16px] leading-relaxed text-fg-muted">
              You pay for each person in the organization. Free, Starter, and
              Team plug in machines you already run. Enterprise is still your
              hardware — with our OS (PWare) on the disk. We never host workers.
            </p>
          </Reveal>

          <Reveal delay={0.08}>
            <div className="mt-14 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
              {PLANS.map((plan) => (
                <article
                  key={plan.id}
                  className={`flex flex-col rounded-panel border bg-sidebar p-6 ${
                    plan.featured ? "border-accent/35" : "border-line-2"
                  }`}
                >
                  <h2 className="text-[17px] font-semibold tracking-tight">
                    {plan.name}
                  </h2>
                  <p className="mt-4 text-[2rem] font-semibold tracking-tight">
                    {plan.price}
                  </p>
                  <p className="mt-1 text-[13.5px] text-fg-subtle">
                    {plan.period}
                  </p>
                  <ul className="mt-6 flex flex-1 flex-col gap-2 text-[14.5px] leading-relaxed text-fg-muted">
                    {plan.items.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                  <a
                    href={plan.href}
                    className={`mt-8 rounded-control px-4 py-2.5 text-center text-[14.5px] font-semibold transition-colors duration-150 active:scale-[0.98] ${
                      plan.featured
                        ? "bg-accent text-accent-on hover:bg-accent-hover"
                        : "border border-line-2 text-fg hover:border-fg-subtle hover:bg-shell"
                    }`}
                  >
                    {plan.cta}
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
