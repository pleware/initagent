import i18n from "../i18n/config";
import { HUB } from "./site";

/** Public Hardware cards. Specs, photos, and prices stay a backlog
 *  fill from `docs/HARDWARE_PWARE.md` — do not copy BOM or margin here.
 *  Product names are brand; the rest of the copy resolves through
 *  i18n when the card renders. */
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
    get role() {
      return i18n.t("hardware.stations.one16.role");
    },
    get items() {
      return [
        i18n.t("hardware.stations.one16.gpu"),
        i18n.t("hardware.stations.preinstalled"),
        i18n.t("hardware.stations.workerReady"),
      ];
    },
  },
  {
    id: "duo-2x16",
    name: "PWARE AI DUO 2×16",
    get role() {
      return i18n.t("hardware.stations.duo2x16.role");
    },
    featured: true,
    get items() {
      return [
        i18n.t("hardware.stations.duo2x16.gpu"),
        i18n.t("hardware.stations.preinstalled"),
        i18n.t("hardware.stations.workerReady"),
      ];
    },
  },
  {
    id: "pro-24-ecc",
    name: "PWARE AI PRO 24 ECC",
    get role() {
      return i18n.t("hardware.stations.pro24ecc.role");
    },
    get items() {
      return [
        i18n.t("hardware.stations.pro24ecc.gpu"),
        i18n.t("hardware.stations.preinstalled"),
        i18n.t("hardware.stations.workerReady"),
      ];
    },
  },
];

export const HARDWARE_ENQUIRE = HUB;
