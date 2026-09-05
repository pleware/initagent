import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";

import en from "./locales/en.json";
import pl from "./locales/pl.json";

i18n.use(LanguageDetector).use(initReactI18next).init({
  resources: {
    en: { translation: en },
    pl: { translation: pl },
  },
  fallbackLng: "en",
  supportedLngs: ["en", "pl"],
  nonExplicitSupportedLngs: true,
  interpolation: { escapeValue: false },
  detection: {
    order: ["querystring", "localStorage", "navigator"],
    caches: ["localStorage"],
    lookupQuerystring: "lng",
    lookupLocalStorage: "i18nextLng",
  },
});

i18n.on("languageChanged", (lng) => {
  document.documentElement.lang = (lng || "en").split("-")[0];
});

export default i18n;
