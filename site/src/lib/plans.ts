import i18n from "../i18n/config";
import { openAppHref } from "./cta";
import {
  PLAN_BY_SLUG,
  PLAN_ORDER,
  type PlanConfig,
  type PlanSlug,
} from "./org-plans.gen";

/** Card copy for `/plans`. Numbers and price come from the catalogue
 *  (`internal/registry/config/catalog.yaml`). Person = org member. A
 *  machine you enroll is yours; we never host workers. Copy resolves
 *  through i18n when the card renders. */
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

/** Cards that get the highlighted treatment on `/plans`. */
const FEATURED: PlanSlug[] = ["starter"];

/** Extra bullets that are pure copy, not derived from the catalogue. */
const EXTRA_ITEM_KEYS: Partial<Record<PlanSlug, string>> = {
  enterprise: "plans.cards.enterprise.extra",
};

function periodKey(id: PlanSlug): string {
  switch (id) {
    case "free":
      return "plans.cards.free.period";
    case "enterprise":
      return "plans.cards.enterprise.period";
    default:
      return "plans.cards.perPersonMonth";
  }
}

function ctaKey(id: PlanSlug): string {
  return id === "enterprise" ? "plans.cards.enterprise.cta" : "nav.openApp";
}

function priceOf(cfg: PlanConfig): string {
  switch (cfg.charge.kind) {
    case "free":
      return "0 €";
    case "contact":
      return i18n.t("plans.price.talkToUs");
    default:
      return `${cfg.charge.eur} €`;
  }
}

function peopleLine(n: number): string | undefined {
  if (n === 1) return i18n.t("plans.cards.people", { count: 1 });
  if (n > 1) return i18n.t("plans.cards.people", { count: n });
  return undefined;
}

function projectLine(n: number): string {
  if (n === 0) return i18n.t("plans.cards.projectsUnlimited");
  return i18n.t("plans.cards.projects", { count: n });
}

function machineLine(slug: PlanSlug, n: number): string {
  if (n === 0) return i18n.t("plans.cards.machinesUnlimited");
  const key =
    slug === "free"
      ? "plans.cards.machinesOnProject"
      : "plans.cards.machinesPerProject";
  return i18n.t(key, { count: n });
}

function itemsOf(
  slug: PlanSlug,
  cfg: PlanConfig,
  extraKey: string | undefined,
): string[] {
  const items: string[] = [];
  const people = peopleLine(cfg.limits.people);
  if (people) items.push(people);
  items.push(projectLine(cfg.limits.projects));
  items.push(machineLine(slug, cfg.limits.workersPerProject));
  if (extraKey) items.push(i18n.t(extraKey));
  return items;
}

export const PLANS: PlanCard[] = PLAN_ORDER.map((id) => ({
  id,
  get name() {
    return i18n.t(`plans.cards.${id}.name`);
  },
  get price() {
    return priceOf(PLAN_BY_SLUG[id]);
  },
  get period() {
    return i18n.t(periodKey(id));
  },
  get featured() {
    return FEATURED.includes(id);
  },
  get cta() {
    return i18n.t(ctaKey(id));
  },
  href: openAppHref("en"),
  get items() {
    return itemsOf(id, PLAN_BY_SLUG[id], EXTRA_ITEM_KEYS[id]);
  },
}));
