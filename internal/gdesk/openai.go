package gdesk

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"net"
	"net/http"
	"strings"
	"time"
)

// ErrDialect marks a binding this build cannot call: a wire dialect with no
// implementation here. It is a configuration fault rather than a provider
// fault, so it is never retried.
var ErrDialect = errors.New("gdesk: dialect")

// ErrRequest marks a call this package refused to make — our own mistake,
// caught before a byte left the box. Also never retried: asking a second
// time with the same broken request wastes the person's turn.
var ErrRequest = errors.New("gdesk: request")

const (
	// defaultChatPath is where the openai dialect answers a turn.
	defaultChatPath = "/chat/completions"

	// maxErrorBody bounds what we read from a refusal. Enough for a provider
	// message, not enough for an HTML error page to become a log entry.
	maxErrorBody = 8 << 10

	// maxSSELine bounds one event. A reply arrives in small deltas; a
	// megabyte line means the far end is not speaking this dialect.
	maxSSELine = 1 << 20
)

// ProviderError is a call the provider refused, with the status kept beside
// the message.
//
// This is fleet.StatusError's sibling for a different remote, and it is not
// shared with it on purpose: the hub client has no retry, while here the
// status *is* the retry decision, and folding both into one type would put
// desk policy in a package that has no use for it.
type ProviderError struct {
	Provider string
	Code     int
	Message  string
}

func (e *ProviderError) Error() string {
	if e.Code == 0 {
		return fmt.Sprintf("gdesk: provider %s failed mid-reply: %s", e.Provider, e.Message)
	}
	return fmt.Sprintf("gdesk: provider %s refused (%d): %s", e.Provider, e.Code, e.Message)
}

// Retryable reports whether asking again could plausibly work.
//
// Code 0 is a failure that arrived *inside* a reply already in progress. It
// is deliberately not retryable here: the caller may have delivered words to
// the scene or the speaker, and re-asking would say them twice. Whether to
// offer a repair line instead is the conversation loop's decision, and it
// needs to be taken with the transcript in hand.
func (e *ProviderError) Retryable() bool {
	switch e.Code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	}
	return e.Code >= 500
}

// Retryable reports whether a failed desk call may be tried again at all,
// before the per-kind windows in retry.go get a say.
//
// Lenient by default: an error nobody classified is most likely a dial or a
// reset, which is exactly what a retry is for. The exceptions are the cases
// where trying again cannot help — a context that is already finished, a
// request we built wrong, and a dialect this build does not speak.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrRequest) || errors.Is(err, ErrDialect) || errors.Is(err, ErrConfig) {
		return false
	}
	var classified interface{ Retryable() bool }
	if errors.As(err, &classified) {
		return classified.Retryable()
	}
	return true
}

// NewHTTPClient is the transport the desk seams use when a caller passes
// none.
//
// There is deliberately no Client.Timeout: a streamed reply is meant to stay
// open for as long as she is talking, and a whole-request deadline would cut
// the longest answers off mid-sentence. The bound that matters is time to the
// first response header, which is where a dead provider actually shows up;
// past that, the caller's context owns the turn.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   4,
			ForceAttemptHTTP2:     true,
		},
	}
}

// NewChat returns the chat seam for a binding, choosing the implementation by
// dialect. Call sites depend on Chat and on configuration, never on which
// vendor answered.
func NewChat(b Binding, hc *http.Client) (Chat, error) {
	if b.Role != RoleChat {
		return nil, fmt.Errorf("%w: NewChat given the %s binding", ErrRequest, b.Role)
	}
	if hc == nil {
		hc = NewHTTPClient()
	}
	switch b.Provider.Shape {
	case ShapeOpenAI:
		return &openAIChat{provider: b.Provider, model: b.Model, http: hc}, nil
	case ShapeAnthropic:
		// Worth stating rather than implementing: a LiteLLM proxy speaks the
		// openai dialect and reaches Claude, so the native Anthropic wire
		// format buys a second parser and no new capability. It arrives if
		// something needs it directly.
		return nil, fmt.Errorf("%w: provider %q speaks %s, which has no chat implementation here — point a %s-shaped proxy at it",
			ErrDialect, b.Provider.ID, ShapeAnthropic, ShapeOpenAI)
	default:
		return nil, fmt.Errorf("%w: provider %q speaks %q", ErrDialect, b.Provider.ID, b.Provider.Shape)
	}
}

// openAIChat answers a turn over the openai dialect.
type openAIChat struct {
	provider Provider
	model    string
	http     *http.Client
}

// chatMessage is one message on the wire.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatBody is the request. Streaming is not optional: the scene shows words
// as they arrive and the speaker starts before the sentence ends, so a
// buffered reply would add the whole generation time to her first syllable.
type chatBody struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatChunk is one streamed event. An error can arrive with a 200 and an open
// stream — LiteLLM and OpenAI both do it — so the shape carries both.
type chatChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Turn implements Chat.
func (c *openAIChat) Turn(ctx context.Context, req ChatRequest) (ChatStream, error) {
	messages := chatMessages(req)
	if len(messages) == 0 {
		return nil, fmt.Errorf("%w: a turn needs at least one line", ErrRequest)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = c.model
	}
	if model == "" {
		return nil, fmt.Errorf("%w: no model, and provider %q binds none", ErrRequest, c.provider.ID)
	}

	body, err := json.Marshal(chatBody{Model: model, Messages: messages, Stream: true})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRequest, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRequest, err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	if c.provider.Key != "" {
		// No header at all when there is no key: a local whisper.cpp or
		// llama.cpp server is an open endpoint, and sending `Bearer ` with
		// nothing after it is how those refuse a request that would work.
		request.Header.Set("Authorization", "Bearer "+c.provider.Key)
	}

	resp, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, c.refusal(resp)
	}

	provider := c.provider.ID
	return func(yield func(ChatDelta, error) bool) {
		// Closing here is what makes ranging to completion enough: a caller
		// that drains the stream, or breaks out of it, releases the
		// connection without knowing there was one.
		defer resp.Body.Close()
		for payload, err := range sseData(resp.Body) {
			if err != nil {
				yield(ChatDelta{}, err)
				return
			}
			var chunk chatChunk
			if err := json.Unmarshal(payload, &chunk); err != nil {
				yield(ChatDelta{}, fmt.Errorf("gdesk: provider %s sent an unreadable event: %w", provider, err))
				return
			}
			if chunk.Error != nil {
				yield(ChatDelta{}, &ProviderError{Provider: provider, Message: chunk.Error.Message})
				return
			}
			for _, choice := range chunk.Choices {
				// The first event of a reply carries the role and no text,
				// and providers pad with empty deltas. Yielding those would
				// make every caller filter them.
				if choice.Delta.Content == "" {
					continue
				}
				if !yield(ChatDelta{Text: choice.Delta.Content}, nil) {
					return
				}
			}
		}
	}, nil
}

// endpoint is where this provider takes a turn.
//
// APIPath overrides the role's default path outright, which means a provider
// entry that sets it serves one role. A provider answering two roles leaves
// it empty and gets both defaults.
func (c *openAIChat) endpoint() string {
	path := c.provider.APIPath
	if path == "" {
		path = defaultChatPath
	}
	return c.provider.BaseURL + path
}

// refusal turns a non-2xx response into an error that says who refused and
// whether asking again is worth it.
func (c *openAIChat) refusal(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	var message string
	if json.Unmarshal(data, &payload) == nil {
		message = strings.TrimSpace(payload.Error.Message)
	}
	if message == "" {
		message = strings.TrimSpace(string(data))
	}
	if message == "" {
		message = resp.Status
	}
	return &ProviderError{Provider: c.provider.ID, Code: resp.StatusCode, Message: message}
}

// chatMessages renders a turn into wire messages.
//
// The word budget is prose, not max_tokens. A token ceiling truncates
// mid-word, and half a spoken sentence is worse than a long one; asking for
// brevity is also the instruction a person would give.
func chatMessages(req ChatRequest) []chatMessage {
	var messages []chatMessage
	system := strings.TrimSpace(req.Brief)
	if req.MaxWords > 0 {
		budget := fmt.Sprintf("Answer in at most %d words.", req.MaxWords)
		if system == "" {
			system = budget
		} else {
			system += "\n\n" + budget
		}
	}
	if system != "" {
		messages = append(messages, chatMessage{Role: "system", Content: system})
	}
	for _, line := range req.Lines {
		text := strings.TrimSpace(line.Text)
		if text == "" {
			// An empty line is noise from a trimmed transcript, and several
			// providers reject empty content outright.
			continue
		}
		role := "user"
		if line.From == LineFromStaff {
			role = "assistant"
		}
		messages = append(messages, chatMessage{Role: role, Content: text})
	}
	return messages
}

// sseData yields the payload of every data line, in order, and stops at the
// terminator. Comments, event names and blank lines are framing rather than
// content, so they never reach the caller.
func sseData(r io.Reader) iter.Seq2[[]byte, error] {
	return func(yield func([]byte, error) bool) {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 8<<10), maxSSELine)
		for scanner.Scan() {
			payload, ok := strings.CutPrefix(strings.TrimSpace(scanner.Text()), "data:")
			if !ok {
				continue
			}
			payload = strings.TrimSpace(payload)
			if payload == "[DONE]" {
				return
			}
			if payload == "" {
				continue
			}
			if !yield([]byte(payload), nil) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			// A stream that ends cleanly without the terminator is treated as
			// a finished reply: it is what a proxy closing an idle connection
			// looks like, and the words already delivered were real.
			yield(nil, fmt.Errorf("gdesk: reading reply: %w", err))
		}
	}
}
