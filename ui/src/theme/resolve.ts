/**
 * Theme resolution: which `data-theme` id a preference turns into.
 *
 * Pure on purpose. Everything here is a function of its arguments, so the
 * awkward cases — a family with no light variant, a stale value in storage,
 * "follow the system" — are decided in one readable place instead of being
 * spread across a provider and a few components. Reading storage, touching
 * the document, and listening for system changes live in `./index.ts`.
 *
 * The palettes themselves are not here. They are CSS, owned by
 * `internal/brand/themes`, and this module only ever names them.
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
  /**
   * Modes with genuine values behind them, most preferred first. A family
   * still answers to both ids so the resolver never special-cases it, but a
   * mode missing here is an alias, not a design.
   */
  modes: readonly [ThemeMode, ...ThemeMode[]]
  /**
   * False while the family is a stub that falls through to the `:root` layer.
   * A picker should not offer it as a real choice yet.
   */
  designed: boolean
  /** Short label for the theme switcher. */
  label: string
}

export const themeFamilies: Readonly<Record<ThemeFamily, FamilySpec>> = {
  // Dark only, and staying that way: it describes a surface that never had a
  // light variant.
  legacy: { modes: ['dark'], designed: true, label: 'Classic' },
  // Five daisyUI 5.7.22 palettes remapped onto --ia-*. Each source theme is
  // one color-scheme, so both ids share the same values (same as legacy).
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

/**
 * Resolve a preference to a concrete id.
 *
 * `system` follows the OS. A family that has no values for the resulting mode
 * falls back to its first designed mode rather than to another family, so
 * asking for a light cockpit never silently changes which palette you get.
 */
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

/**
 * Whether the OS preference has any effect on this family. A picker can use
 * this to explain why "system" is doing nothing on a dark-only family.
 */
export function honoursSystemMode(family: ThemeFamily): boolean {
  return themeFamilies[family].modes.length > 1
}

/**
 * Narrow an unknown value — a parsed storage entry, a query parameter — to a
 * usable preference. Anything unrecognised falls back field by field, so one
 * bad half does not discard the other.
 */
export function parsePreference(value: unknown): ThemePreference {
  if (typeof value !== 'object' || value === null) return defaultPreference

  const { family, mode } = value as Partial<ThemePreference>

  return {
    family: isThemeFamily(family) ? family : defaultPreference.family,
    mode: isThemeChoice(mode) ? mode : defaultPreference.mode,
  }
}
