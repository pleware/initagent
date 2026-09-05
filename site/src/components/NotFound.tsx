import { ROUTES } from "../lib/routes";

export function NotFound() {
  return (
    <main>
      <section className="pt-24 pb-24 lg:pt-32 lg:pb-32">
        <div className="mx-auto max-w-[1240px] px-5 lg:px-8">
          <h1 className="text-[2rem] leading-[1.1] font-semibold tracking-tight">
            This page is not here.
          </h1>
          <p className="mt-5 max-w-[46ch] text-[16px] leading-relaxed text-fg-muted">
            The address does not match a page on this site.
          </p>
          <a
            href={ROUTES.home}
            className="mt-8 inline-block rounded-control bg-accent px-4 py-2.5 text-[14.5px] font-semibold text-accent-on hover:bg-accent-hover"
          >
            Back to initagent
          </a>
        </div>
      </section>
    </main>
  );
}
