// Package desk is the conversation seam for the front desk: the interfaces
// Adam and Ania speak through, and the configuration that binds each of them
// to a provider (drafts 53, 41, and workspace docs/DESK-CONVERSATION.md §10).
//
// It runs on the connector, on the box — never in the Electron shell. The
// shell owns the microphone and the speaker, which are devices; keys are
// secrets the HUD must never hold, a closed window must not kill a
// transcription mid-turn, and retry logic must not exist twice in two
// languages.
//
// Three seams rather than one SDK, because "OpenAI SDK or Anthropic SDK"
// does not answer the voice question: Anthropic ships neither speech to text
// nor text to speech. Whether one provider serves all three is
// configuration. What we depend on is a wire dialect (Shape), so one
// OpenAI-compatible implementation reaches OpenAI, a LiteLLM proxy, Groq,
// vLLM, a local whisper.cpp server and Piper behind a thin HTTP shim.
//
// This package holds no loop, no socket and no model prompt. It is the
// vocabulary those things are written against.
package desk

import (
	"context"
	"iter"
)

// Role names one job at the desk. Roles are named rather than inferred from
// the provider, which is what lets ElevenLabs be swapped for Piper without
// touching the conversation loop.
type Role string

const (
	// RoleChat answers a person: it turns an utterance into words back.
	RoleChat Role = "desk.chat"

	// RoleSTT hears: it turns captured audio into text.
	RoleSTT Role = "desk.stt"

	// RoleTTS speaks: it turns text into audio for the shell to play.
	RoleTTS Role = "desk.tts"
)

// Roles lists every role in a fixed order, so a caller reporting
// configuration never depends on map iteration order.
var Roles = []Role{RoleChat, RoleSTT, RoleTTS}

// LineSource says who spoke a line of the conversation so far. The staff
// member's own replies come back as LineFromStaff, which is what keeps a
// thread readable when it is replayed after a reconnect.
type LineSource string

const (
	// LineFromPerson is the human at the desk.
	LineFromPerson LineSource = "person"

	// LineFromStaff is Adam or Ania.
	LineFromStaff LineSource = "staff"
)

// Line is one line of conversation as a model sees it.
type Line struct {
	From LineSource
	Text string
}

// ChatRequest is one turn. Brief carries who the staff member is — the Big
// Five baseline, the prose rules and the name, all of which are hub rows —
// already rendered to text by the caller, because this package must not know
// how personality is stored.
type ChatRequest struct {
	Model    string
	Brief    string
	Lines    []Line
	MaxWords int
}

// ChatDelta is one piece of a reply as it arrives. A delta is text today; it
// stays a struct so tool calls can be added without changing the seam.
type ChatDelta struct {
	Text string
}

// ChatStream yields the reply in order and ends after the last delta, or
// yields one error. Ranging to completion is what releases the underlying
// connection, so a caller that breaks early must still drain or cancel the
// context it passed.
type ChatStream = iter.Seq2[ChatDelta, error]

// Chat answers a person.
type Chat interface {
	Turn(ctx context.Context, req ChatRequest) (ChatStream, error)
}

// AudioFormat is the encoding of a block of audio on this seam. The shell
// captures PCM; a provider may hand back something compressed.
type AudioFormat string

const (
	// AudioPCM16 is signed 16-bit little-endian PCM, which is what the
	// microphone path produces.
	AudioPCM16 AudioFormat = "pcm16"

	// AudioWAV is PCM with a RIFF header, which most transcribers accept as
	// an upload without a format argument.
	AudioWAV AudioFormat = "wav"

	// AudioMP3 is what several speech providers return by default.
	AudioMP3 AudioFormat = "mp3"
)

// AudioRef is captured speech handed to a transcriber. Language is a hint,
// not a promise: the desk runs a multilingual model on purpose, so a person
// switching mid-sentence is heard rather than corrected.
type AudioRef struct {
	Format       AudioFormat
	SampleRateHz int
	Language     string
	Bytes        []byte
}

// Transcript is what the desk heard.
type Transcript struct {
	Text     string
	Language string
}

// Transcriber hears.
type Transcriber interface {
	Transcribe(ctx context.Context, audio AudioRef) (Transcript, error)
}

// SpeechRequest is one line to say out loud. VoiceID, Language and Rate are
// identity and come from the hub — who she sounds like is who she is, while
// the engine that renders it is inventory on this box.
type SpeechRequest struct {
	Model    string
	VoiceID  string
	Language string
	Rate     float64
	Text     string
}

// AudioStream is rendered speech. Format is known before the first chunk so
// the shell can open the output device once, and Chunks yields bytes in
// order.
type AudioStream struct {
	Format       AudioFormat
	SampleRateHz int
	Chunks       iter.Seq2[[]byte, error]
}

// Speaker speaks.
type Speaker interface {
	Speak(ctx context.Context, req SpeechRequest) (AudioStream, error)
}
