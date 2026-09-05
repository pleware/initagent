import { HUB } from "./site";

/** Public Hardware cards. Specs, photos, and prices stay a backlog
 *  fill from `docs/HARDWARE_PWARE.md` — do not copy BOM or margin here. */
export type Station = {
  id: string;
  name: string;
  role: string;
  featured?: boolean;
  items: string[];
};

export const STATIONS: Station[] = [
  {
    id: "one-16",
    name: "PWARE AI ONE 16",
    role: "One GPU. A first box.",
    items: [
      "16 GB GPU memory on one card",
      "PWare OS preinstalled",
      "initagent worker ready to join a hub",
    ],
  },
  {
    id: "duo-2x16",
    name: "PWARE AI DUO 2×16",
    role: "Two cards. The station we recommend.",
    featured: true,
    items: [
      "2×16 GB GPU memory, not one shared 32 GB pool",
      "PWare OS preinstalled",
      "initagent worker ready to join a hub",
    ],
  },
  {
    id: "pro-24-ecc",
    name: "PWARE AI PRO 24 ECC",
    role: "One professional GPU, ECC memory.",
    items: [
      "24 GB ECC GPU memory on one card",
      "PWare OS preinstalled",
      "initagent worker ready to join a hub",
    ],
  },
];

export const HARDWARE_ENQUIRE = HUB;
