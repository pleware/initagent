import { HUB } from "./site";

/** Must match `internal/orgplan` (draft 48). Person = org member. A
 *  machine you enroll is yours; we never host workers. */
export const PERSON_USD = 5;

export type PlanCard = {
  id: string;
  name: string;
  price: string;
  period: string;
  featured?: boolean;
  cta: string;
  href: string;
  items: string[];
};

export const PLANS: PlanCard[] = [
  {
    id: "free",
    name: "Free",
    price: "$0",
    period: "one person",
    cta: "Open app",
    href: HUB,
    items: [
      "1 person",
      "1 project",
      "2 of your machines on that project",
    ],
  },
  {
    id: "starter",
    name: "Starter",
    price: `$${PERSON_USD}`,
    period: "per person / month",
    featured: true,
    cta: "Open app",
    href: HUB,
    items: [
      "2 projects",
      "3 of your machines per project",
    ],
  },
  {
    id: "team",
    name: "Team",
    price: `$${PERSON_USD}`,
    period: "per person / month",
    cta: "Open app",
    href: HUB,
    items: [
      "5 projects",
      "5 of your machines per project",
    ],
  },
  {
    id: "enterprise",
    name: "Enterprise",
    price: "Talk to us",
    period: "no public price",
    cta: "Contact sales",
    href: HUB,
    items: [
      "No project cap",
      "No machine cap",
      "Your machines, running PWare",
    ],
  },
];
