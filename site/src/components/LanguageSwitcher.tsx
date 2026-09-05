import { useTranslation } from "react-i18next";
import { LanguageSwitcher as SharedLanguageSwitcher } from "../../../web/LanguageSwitcher.tsx";
import { resolveLocale, type Locale } from "../../../web/locale.ts";

export default function LanguageSwitcher({
  className,
  size = "compact",
}: {
  className?: string;
  size?: "compact" | "nav";
}) {
  const { i18n } = useTranslation();
  const value = resolveLocale(i18n.resolvedLanguage || i18n.language);

  const handleChange = (locale: Locale) => {
    void i18n.changeLanguage(locale);
    document.documentElement.lang = locale;
  };

  return (
    <SharedLanguageSwitcher
      value={value}
      onChange={handleChange}
      className={className}
      size={size}
    />
  );
}
