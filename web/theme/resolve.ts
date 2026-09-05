/**
 * Theme resolution: which `data-theme` id a preference turns into.
 *
 * Pure on purpose. Palettes are CSS in `internal/brand/themes`.
 */

export type ThemeFamily =
  | 'legacy'
  | 'corporate'
  | 'night'
  | 'dim'
  | 'nord'
  | 'sunset'
  | 'default'
  | 'enterprise'

/** A mode a stylesheet can actually be in. */
export type ThemeMode = 'light' | 'dark'

/** A mode a person can choose, including deferring to the OS. */
export type ThemeChoice = ThemeMode | 'system'

/** The value written to `data-theme`, matching a selector in the CSS. */
export type ThemeId = `${ThemeFamily}-${ThemeMode}`

export type ThemePreference = {
  family: ThemeFamily
  mode: ThemeChoice
}

type FamilySpec = {
  modes: readonly [ThemeMode, ...ThemeMode[]]
  designed: boolean
  label: string
}

export const themeFamilies: Readonly<Record<ThemeFamily, FamilySpec>> = {
  legacy: { modes: ['dark'], designed: true, label: 'Classic' },
  corporate: { modes: ['light'], designed: true, label: 'Corporate' },
  night: { modes: ['dark'], designed: true, label: 'Night' },
  dim: { modes: ['dark'], designed: true, label: 'Dim' },
  nord: { modes: ['light'], designed: true, label: 'Nord' },
  sunset: { modes: ['dark'], designed: true, label: 'Sunset' },
  default: { modes: ['dark', 'light'], designed: false, label: 'Default' },
  enterprise: { modes: ['dark', 'light'], designed: false, label: 'Enterprise' },
}

/** Families the switcher may offer — designed palettes only, registry order. */
export function pickerFamilies(): ThemeFamily[] {
  return (Object.keys(themeFamilies) as ThemeFamily[]).filter(
    (family) => themeFamilies[family].designed,
  )
}

export const defaultPreference: ThemePreference = {
  family: 'legacy',
  mode: 'system',
}

export function isThemeFamily(value: unknown): value is ThemeFamily {
  return typeof value === 'string' && value in themeFamilies
}

export function isThemeChoice(value: unknown): value is ThemeChoice {
  return value === 'light' || value === 'dark' || value === 'system'
}

export function resolveThemeId(
  preference: ThemePreference,
  systemPrefersDark: boolean,
): ThemeId {
  const family = isThemeFamily(preference.family)
    ? preference.family
    : defaultPreference.family

  const wanted: ThemeMode =
    preference.mode === 'system'
      ? systemPrefersDark
        ? 'dark'
        : 'light'
      : preference.mode

  const { modes } = themeFamilies[family]
  const mode = modes.includes(wanted) ? wanted : modes[0]

  return `${family}-${mode}`
}

export function honoursSystemMode(family: ThemeFamily): boolean {
  return themeFamilies[family].modes.length > 1
}

export function parsePreference(value: unknown): ThemePreference {
  if (typeof value !== 'object' || value === null) return defaultPreference

  const { family, mode } = value as Partial<ThemePreference>

  return {
    family: isThemeFamily(family) ? family : defaultPreference.family,
    mode: isThemeChoice(mode) ? mode : defaultPreference.mode,
  }
}
