# Internationalization (i18n)

This directory contains the i18n setup for the initagent UI.

## Current state

- **English (en)**: Complete ✅
- **Polish (pl)**: Not yet implemented ⏳

## Adding a new language

1. Create a new directory under `locales/` (e.g., `locales/pl/`)
2. Copy `locales/en/translation.json` to the new directory
3. Translate all strings in the new `translation.json`
4. Add the language to `config.ts`:
   ```ts
   import plTranslation from './locales/pl/translation.json';
   
   const resources = {
     en: { translation: enTranslation },
     pl: { translation: plTranslation }, // Add this line
   };
   ```
5. Add the language to `LanguageSwitcher.tsx`:
   ```ts
   const LANGUAGES = [
     { code: 'en', name: 'English', flag: '🇬🇧' },
     { code: 'pl', name: 'Polski', flag: '🇵🇱' }, // Uncomment this
   ];
   ```

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
- `devices.*` — Device management strings
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

- The i18n setup is **ready** but only English is available
- Polish translations can be added in one PR later
- All new UI strings should go through `t()` from the start
