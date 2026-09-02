import { useTranslation } from 'react-i18next';

const LANGUAGES = [
  { code: 'en', name: 'English', flag: '🇬🇧' },
  // Uncomment when Polish translations are ready:
  // { code: 'pl', name: 'Polski', flag: '🇵🇱' },
] as const;

export default function LanguageSwitcher() {
  const { i18n } = useTranslation();

  const handleLanguageChange = (langCode: string) => {
    i18n.changeLanguage(langCode);
  };

  return (
    <div className="relative inline-block">
      <select
        value={i18n.language}
        onChange={(e) => handleLanguageChange(e.target.value)}
        className="appearance-none rounded-lg border border-white/[0.07] bg-white/[0.025] px-3 py-2 pr-8 text-sm text-white hover:bg-white/[0.05] focus:outline-none focus:ring-2 focus:ring-blue-500"
        aria-label="Select language"
      >
        {LANGUAGES.map((lang) => (
          <option key={lang.code} value={lang.code}>
            {lang.flag} {lang.name}
          </option>
        ))}
      </select>
      <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center pr-2">
        <svg className="h-4 w-4 text-zinc-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </div>
    </div>
  );
}
