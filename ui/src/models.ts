import type { Model, Purpose } from './types'

// The nine purposes a pinned model serves, in select order. Both the models
// admin page and the per-box override panel iterate this list, so it lives
// here rather than in either page. The labels sit in i18n under purpose.*
// and stay identical in both locales.
export const PURPOSES: Purpose[] = [
  'persona',
  'worker',
  'narrator',
  'embedding',
  'stt',
  'vad',
  'turn',
  'tts',
  'classification',
]

// GENERATIVE_PURPOSES names the slots an LLM serves through llama.cpp — the
// only purposes that carry generation limits. embedding/stt/vad/turn/tts are
// non-generative engines and carry none.
export const GENERATIVE_PURPOSES: Purpose[] = ['persona', 'worker', 'narrator']

// ENGINES is the order the registry is grouped in: llama.cpp first — an empty
// `engine` on the wire means it, so it holds most pins — then the audio.cpp
// compartment the same box runs beside it, then the three engines that each
// answer one role. A pin's engine is what decides which program opens its
// bytes, so this grouping shows a fact the hub already holds rather than
// classifying anything a second time.
export const ENGINES = ['llama.cpp', 'audio.cpp', 'whisper.cpp', 'laya', 'piper']

// engineOf answers which program a pin belongs to. Empty on the wire means
// llama.cpp: the hub writes the field only for engines that are not the
// default, so every pin predating it lands where it always landed.
export function engineOf(model: Model): string {
  return model.engine || 'llama.cpp'
}

// engineKey is an engine's wire name as an i18n key suffix — a dot inside a
// key would nest it, so `audio.cpp` reads `engine.audioCpp`.
export function engineKey(engine: string): string {
  return engine.replace(/\.cpp$/, 'Cpp')
}

// engineGroups splits a catalogue into one group per engine, in ENGINES order,
// appending any engine the wire carries that this list does not know. Nothing
// is dropped: an engine nobody listed shows up under its own raw name, because
// a pin that quietly vanished from the registry is exactly the failure this
// page exists to prevent.
export function engineGroups(models: Model[]): { engine: string; rows: Model[] }[] {
  const byEngine = new Map<string, Model[]>()
  for (const model of models) {
    const engine = engineOf(model)
    const rows = byEngine.get(engine)
    if (rows) {
      rows.push(model)
    } else {
      byEngine.set(engine, [model])
    }
  }
  const known = ENGINES.filter((engine) => byEngine.has(engine))
  const rest = [...byEngine.keys()].filter((engine) => !ENGINES.includes(engine)).sort()
  return [...known, ...rest].map((engine) => ({ engine, rows: byEngine.get(engine) ?? [] }))
}

// modelLabel renders one picker line: the pin id, and its quantisation when
// the pin carries one (embedding and stt pins do not).
export function modelLabel(model: Model): string {
  return model.quant ? `${model.id} · ${model.quant}` : model.id
}

// modelPurposes answers the eligibility set a pin carries. Older hubs (or a
// stale cache) may send only the single `purpose`; that falls back to a
// one-entry set.
export function modelPurposes(model: Model): Purpose[] {
  return model.purposes?.length ? model.purposes : [model.purpose]
}

// modelServes reports whether a pin may serve a given role — the picker's
// filter, and the mirror of the hub's assignment validation.
export function modelServes(model: Model, purpose: Purpose): boolean {
  return modelPurposes(model).includes(purpose)
}
