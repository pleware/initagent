import type { Model, Purpose } from './types'

// The seven purposes a pinned model serves, in select order. Both the models
// admin page and the per-box override panel iterate this list, so it lives
// here rather than in either page. The labels sit in i18n under purpose.*
// and stay identical in both locales.
export const PURPOSES: Purpose[] = ['persona', 'worker', 'narrator', 'embedding', 'stt', 'vad', 'tts']

// GENERATIVE_PURPOSES names the slots an LLM serves through llama.cpp — the
// only purposes that carry generation limits. embedding/stt/vad/tts are
// non-generative engines and carry none.
export const GENERATIVE_PURPOSES: Purpose[] = ['persona', 'worker', 'narrator']

// modelLabel renders one picker line: the pin id, and its quantisation when
// the pin carries one (embedding and stt pins do not).
export function modelLabel(model: Model): string {
  return model.quant ? `${model.id} · ${model.quant}` : model.id
}
