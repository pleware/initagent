import { useState } from "react";
import {
  currentPreference,
  pickerFamilies,
  setTheme,
  themeFamilies,
  type ThemeFamily,
} from "../../../ui/src/theme/index.ts";

export function ThemeSwitcher({ className = "" }: { className?: string }) {
  const designed = pickerFamilies();
  const stored = currentPreference().family;
  const initial = designed.includes(stored) ? stored : "legacy";
  const [family, setFamily] = useState<ThemeFamily>(initial);

  return (
    <label className={className}>
      <span className="sr-only">Theme</span>
      <select
        value={family}
        onChange={(event) => {
          const next = event.target.value as ThemeFamily;
          setFamily(next);
          setTheme({ family: next });
        }}
        className="rounded-control border border-line bg-ink-900 px-3 py-2 text-[13.5px] text-fg-muted transition-colors hover:bg-ink-850 hover:text-fg focus:border-beacon focus:outline-none"
      >
        {designed.map((id) => (
          <option key={id} value={id}>
            {themeFamilies[id].label}
          </option>
        ))}
      </select>
    </label>
  );
}
