import { HUB } from "./site";
import {
  PERSON_USD,
  PLAN_BY_SLUG,
  PLAN_ORDER,
  type PlanConfig,
  type PlanSlug,
} from "./org-plans.gen";

/** Card copy for `/plans`. Numbers and price come from the catalogue
 *  (`internal/registry/config/catalog.yaml`). Person = org member. A
 *  machine you enroll is yours; we never host workers. */
export type PlanCard = {
  id: PlanSlug;
  name: string;
  price: string;
  period: string;
  featured?: boolean;
  cta: string;
  href: string;
  items: string[];
};

type CardCopy = {
  name: string;
  period: string;
  featured?: boolean;
  cta: string;
  extra?: string[];
};

const COPY: Record<PlanSlug, CardCopy> = {
  free: { name: "Free", period: "one person", cta: "Open app" },
  starter: {
    name: "Starter",
    period: "per person / month",
    featured: true,
    cta: "Open app",
  },
  team: { name: "Team", period: "per person / month", cta: "Open app" },
  enterprise: {
    name: "Enterprise",
    period: "no public price",
    cta: "Contact sales",
    extra: ["Your machines, running PWare"],
  },
};

function priceOf(cfg: PlanConfig): string {
  switch (cfg.charge.kind) {
    case "free":
      return "$0";
    case "contact":
      return "Talk to us";
    default:
      return `$${PERSON_USD}`;
  }
}

function peopleLine(n: number): string | undefined {
  if (n === 1) return "1 person";
  if (n > 1) return `${n} people`;
  return undefined;
}

function projectLine(n: number): string {
  if (n === 0) return "No project cap";
  if (n === 1) return "1 project";
  return `${n} projects`;
}

function machineLine(slug: PlanSlug, n: number): string {
  if (n === 0) return "No machine cap";
  if (slug === "free") {
    if (n === 1) return "1 of your machines on that project";
    return `${n} of your machines on that project`;
  }
  if (n === 1) return "1 of your machines per project";
  return `${n} of your machines per project`;
}

function itemsOf(slug: PlanSlug, cfg: PlanConfig, extra: string[] | undefined): string[] {
  const items: string[] = [];
  const people = peopleLine(cfg.limits.people);
  if (people) items.push(people);
  items.push(projectLine(cfg.limits.projects));
  items.push(machineLine(slug, cfg.limits.workersPerProject));
  if (extra) items.push(...extra);
  return items;
}

export const PLANS: PlanCard[] = PLAN_ORDER.map((id) => {
  const cfg = PLAN_BY_SLUG[id];
  const copy = COPY[id];
  return {
    id,
    name: copy.name,
    price: priceOf(cfg),
    period: copy.period,
    featured: copy.featured,
    cta: copy.cta,
    href: HUB,
    items: itemsOf(id, cfg, copy.extra),
  };
});
