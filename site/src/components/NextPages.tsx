import { useTranslation } from "react-i18next";
import { Reveal } from "../lib/reveal";
import { ROUTES } from "../lib/routes";

export function NextPages() {
  const { t } = useTranslation();

  const doors = [
    {
      href: ROUTES.plans,
      title: t("nav.plans"),
      body: t("nextPages.plansBody"),
    },
    {
      href: ROUTES.developers,
      title: t("nav.developers"),
      body: t("nextPages.developersBody"),
    },
    {
      href: ROUTES.hardware,
      title: t("nav.hardware"),
      body: t("nextPages.hardwareBody"),
    },
  ];

  return (
    <section className="border-t border-line-1 py-24 lg:py-32">
      <div className="mx-auto max-w-[1240px] px-5 lg:px-8">
        <Reveal>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            {doors.map((door) => (
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
