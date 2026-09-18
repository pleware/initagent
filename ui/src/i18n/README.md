# Internationalization (i18n)

This directory contains the i18n setup for the initagent UI.

## Current state

- **English (en)**: Complete ✅ (in `locales/en/translation.json`)
- **Polish (pl)**: Complete ✅ (in `locales/pl/translation.json`)

## Adding a new language

1. Create a new directory under `locales/` (e.g., `locales/de/`)
2. Copy `locales/en/translation.json` to the new directory
3. Translate all strings in the new `translation.json`
4. Register the language in `config.ts`:
   ```ts
   import deTranslation from './locales/de/translation.json';

   const resources = {
     en: { translation: enTranslation },
     pl: { translation: plTranslation },
     de: { translation: deTranslation },
   };

   // and add it to supportedLngs:
   supportedLngs: ['en', 'pl', 'de'],
   ```
5. Register the language in `web/locale.ts`:
   ```ts
   export const LOCALES = [
     { value: 'en', label: 'EN' },
     { value: 'pl', label: 'PL' },
     { value: 'de', label: 'DE' },
   ] as const
   ```
   `resolveLocale` collapses a BCP 47 tag onto the supported languages; extend it to return the new code. The `LanguageSwitcher` components are driven by `LOCALES`, so no other edit is needed.

## Usage in components

```tsx
import { useTranslation } from 'react-i18next';

function MyComponent() {
  const { t } = useTranslation();
  
  return (
    <div>
      <h1>{t('dashboard.title')}</h1>
      <p>{t('dashboard.subtitle')}</p>
    </div>
  );
}
```

### With interpolation

```tsx
{t('validation.minLength', { min: 8 })}
// Output: "Must be at least 8 characters"
```

## Translation keys structure

- `common.*` — Generic UI elements (buttons, actions)
- `dashboard.*` — Dashboard-specific strings
- `connectors.*` — Connector management strings
- `settings.*` — Settings page strings
- `auth.*` — Authentication strings
- `errors.*` — Error messages
- `validation.*` — Form validation messages
- `nav.*` — Navigation menu items

## Language detection

The app detects language in this order:
1. `localStorage` value (`i18nextLng`)
2. Browser language (`navigator.language`)
3. Fallback to English (`en`)

## Guidelines

- Keep translation keys in English, lowercase, with dots for nesting
- Group related translations under the same prefix
- Always provide an English translation
- Use interpolation for dynamic values (`{{variable}}`)
- Keep strings concise and clear
- Consider context when translating (UI, errors, technical terms)

## Notes

- Both English and Polish are shipped; the language switcher stores the choice on the signed-in account
- All new UI strings should go through `t()` from the start
