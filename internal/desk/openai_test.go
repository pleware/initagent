package desk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// openAIBinding is a chat binding pointed at a test server.
func openAIBinding(baseURL, key string) Binding {
	return Binding{
		Role:     RoleChat,
		Provider: Provider{ID: "test-main", Shape: ShapeOpenAI, BaseURL: baseURL, Key: key},
		Model:    "gpt-4o-mini",
	}
}

// sse writes one event in the dialect a provider streams.
func sse(w http.ResponseWriter, payload string) {
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "data: %s\n\n", payload)
	w.(http.Flusher).Flush()
}

func TestNewChatChoosesByDialect(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		binding Binding
		wantErr error
		wantIn  string
	}{
		{
			name:    "the openai dialect is implemented",
			binding: openAIBinding("https://example.test/v1", "k"),
		},
		{
			name: "anthropic names the proxy that reaches it",
			binding: Binding{
				Role:     RoleChat,
				Provider: Provider{ID: "claude", Shape: ShapeAnthropic},
				Model:    "claude-sonnet",
			},
			wantErr: ErrDialect,
			wantIn:  "proxy",
		},
		{
			name: "an unknown dialect names the provider",
			binding: Binding{
				Role:     RoleChat,
				Provider: Provider{ID: "odd", Shape: Shape("mystery")},
				Model:    "m",
			},
			wantErr: ErrDialect,
			wantIn:  `"odd"`,
		},
		{
			name:    "the wrong role is our mistake, not the provider's",
			binding: Binding{Role: RoleTTS, Provider: Provider{ID: "p", Shape: ShapeOpenAI}, Model: "m"},
			wantErr: ErrRequest,
			wantIn:  string(RoleTTS),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			chat, err := NewChat(tc.binding, nil)
			if tc.wantErr == nil {
				if err != nil || chat == nil {
					t.Fatalf("NewChat = %v, %v, want a seam", chat, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("NewChat error = %v, want %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error %q does not mention %q", err, tc.wantIn)
			}
		})
	}
}

func TestTurnStreamsTheReply(t *testing.T) {
	t.Parallel()
	var gotBody chatBody
	var gotAuth, gotAccept, gotPath, gotMethod string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		// The shapes a real stream mixes: a role-only opener, padding, an
		// event name, a comment, and CRLF framing.
		sse(w, `{"choices":[{"delta":{"role":"assistant"}}]}`)
		sse(w, `{"choices":[{"delta":{"content":"Dzień "}}]}`)
		fmt.Fprint(w, ": keep-alive\r\nevent: ping\r\n\r\n")
		sse(w, `{"choices":[{"delta":{"content":"dobry"}}]}`)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	chat, err := NewChat(openAIBinding(server.URL, "sk-test"), server.Client())
	if err != nil {
		t.Fatalf("NewChat: %v", err)
	}
	stream, err := chat.Turn(context.Background(), ChatRequest{
		Brief:    "You are Ania.",
		Lines:    []Line{{From: LineFromPerson, Text: "cześć"}},
		MaxWords: 30,
	})
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}

	var said strings.Builder
	for delta, err := range stream {
		if err != nil {
			t.Fatalf("stream: %v", err)
		}
		said.WriteString(delta.Text)
	}
	if got := said.String(); got != "Dzień dobry" {
		t.Errorf("reply = %q, want %q", got, "Dzień dobry")
	}
	if gotMethod != http.MethodPost || gotPath != defaultChatPath {
		t.Errorf("called %s %s, want POST %s", gotMethod, gotPath, defaultChatPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotAccept != "text/event-stream" {
		t.Errorf("Accept = %q", gotAccept)
	}
	if !gotBody.Stream {
		t.Error("stream is not optional: the scene shows words as they arrive")
	}
	if gotBody.Model != "gpt-4o-mini" {
		t.Errorf("model = %q, want the bound one", gotBody.Model)
	}
}

func TestTurnPrefersTheRequestedModel(t *testing.T) {
	t.Parallel()
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body chatBody
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	chat, _ := NewChat(openAIBinding(server.URL, "k"), server.Client())
	stream, err := chat.Turn(context.Background(), ChatRequest{
		Model: "cheap-for-background",
		Lines: []Line{{From: LineFromPerson, Text: "hej"}},
	})
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	for range stream {
	}
	if gotModel != "cheap-for-background" {
		t.Errorf("model = %q, want the request to win over the binding", gotModel)
	}
}

func TestTurnRefusesBeforeTheWire(t *testing.T) {
	t.Parallel()
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()

	cases := []struct {
		name    string
		binding Binding
		req     ChatRequest
	}{
		{
			name:    "no lines is nothing to answer",
			binding: openAIBinding(server.URL, "k"),
			req:     ChatRequest{Brief: "", Lines: []Line{{From: LineFromPerson, Text: "  "}}},
		},
		{
			name: "no model anywhere",
			binding: Binding{
				Role:     RoleChat,
				Provider: Provider{ID: "p", Shape: ShapeOpenAI, BaseURL: server.URL},
			},
			req: ChatRequest{Lines: []Line{{From: LineFromPerson, Text: "hej"}}},
		},
		{
			name:    "a base URL that is not a URL",
			binding: Binding{Role: RoleChat, Provider: Provider{ID: "p", Shape: ShapeOpenAI, BaseURL: "http://\x7f"}, Model: "m"},
			req:     ChatRequest{Lines: []Line{{From: LineFromPerson, Text: "hej"}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chat, err := NewChat(tc.binding, server.Client())
			if err != nil {
				t.Fatalf("NewChat: %v", err)
			}
			stream, err := chat.Turn(context.Background(), tc.req)
			if !errors.Is(err, ErrRequest) {
				t.Fatalf("Turn error = %v, want ErrRequest", err)
			}
			if stream != nil {
				t.Error("a refused turn returns no stream")
			}
			if Retryable(err) {
				t.Error("our own broken request must not be retried")
			}
		})
	}
	if called {
		t.Error("a refused turn reached the provider")
	}
}

func TestTurnReportsAnUnreachableProvider(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client := server.Client()
	base := server.URL
	server.Close()

	chat, _ := NewChat(openAIBinding(base, "k"), client)
	_, err := chat.Turn(context.Background(), ChatRequest{
		Lines: []Line{{From: LineFromPerson, Text: "hej"}},
	})
	if err == nil {
		t.Fatal("Turn to a closed provider returned no error")
	}
	if errors.Is(err, ErrRequest) {
		// A box that cannot be reached is not a request we built wrong, and
		// calling it one would stop the retry that fixes it.
		t.Errorf("error = %v, want a transport failure rather than ErrRequest", err)
	}
	if !Retryable(err) {
		t.Error("an unreachable provider is the retryable case")
	}
}

func TestTurnReportsARefusal(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		status    int
		body      string
		wantIn    string
		wantRetry bool
	}{
		{
			name:      "rate limit is worth another attempt",
			status:    http.StatusTooManyRequests,
			body:      `{"error":{"message":"slow down"}}`,
			wantIn:    "slow down",
			wantRetry: true,
		},
		{
			name:   "a bad request is not",
			status: http.StatusBadRequest,
			body:   `{"error":{"message":"context length exceeded"}}`,
			wantIn: "context length exceeded",
		},
		{
			name:      "a gateway failure is",
			status:    http.StatusBadGateway,
			body:      "<html>upstream down</html>",
			wantIn:    "upstream down",
			wantRetry: true,
		},
		{
			name:   "an empty body still names the status",
			status: http.StatusUnauthorized,
			body:   "",
			wantIn: "401",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()

			chat, _ := NewChat(openAIBinding(server.URL, "k"), server.Client())
			_, err := chat.Turn(context.Background(), ChatRequest{
				Lines: []Line{{From: LineFromPerson, Text: "hej"}},
			})
			var refusal *ProviderError
			if !errors.As(err, &refusal) {
				t.Fatalf("Turn error = %v, want a ProviderError", err)
			}
			if refusal.Code != tc.status || refusal.Provider != "test-main" {
				t.Errorf("refusal = %+v, want status %d from test-main", refusal, tc.status)
			}
			if !strings.Contains(refusal.Error(), tc.wantIn) {
				t.Errorf("error %q does not mention %q", refusal, tc.wantIn)
			}
			if got := Retryable(err); got != tc.wantRetry {
				t.Errorf("Retryable = %v, want %v", got, tc.wantRetry)
			}
		})
	}
}

func TestTurnReportsAFailureMidReply(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sse(w, `{"choices":[{"delta":{"content":"zaraz"}}]}`)
		sse(w, `{"error":{"message":"upstream closed"}}`)
	}))
	defer server.Close()

	chat, _ := NewChat(openAIBinding(server.URL, "k"), server.Client())
	stream, err := chat.Turn(context.Background(), ChatRequest{
		Lines: []Line{{From: LineFromPerson, Text: "hej"}},
	})
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}

	var said string
	var streamErr error
	for delta, err := range stream {
		if err != nil {
			streamErr = err
			break
		}
		said += delta.Text
	}
	if said != "zaraz" {
		t.Errorf("delivered %q, want the words that did arrive", said)
	}
	var refusal *ProviderError
	if !errors.As(streamErr, &refusal) || refusal.Code != 0 {
		t.Fatalf("stream error = %v, want a mid-reply ProviderError", streamErr)
	}
	if Retryable(streamErr) {
		t.Error("re-asking after partial words would say them twice")
	}
	if !strings.Contains(refusal.Error(), "mid-reply") {
		t.Errorf("error %q does not say when it happened", refusal)
	}
}

func TestTurnReportsAnUnreadableEvent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sse(w, `{"choices":[{"delta":`)
	}))
	defer server.Close()

	chat, _ := NewChat(openAIBinding(server.URL, ""), server.Client())
	stream, err := chat.Turn(context.Background(), ChatRequest{
		Lines: []Line{{From: LineFromPerson, Text: "hej"}},
	})
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	var streamErr error
	for _, err := range stream {
		streamErr = err
	}
	if streamErr == nil || !strings.Contains(streamErr.Error(), "unreadable") {
		t.Fatalf("stream error = %v, want an unreadable event", streamErr)
	}
}

// closeCounter records that a drained stream released its connection, which
// is the whole reason ChatStream is an iterator rather than a reader.
type closeCounter struct {
	io.ReadCloser
	closes *atomic.Int32
}

func (c closeCounter) Close() error {
	c.closes.Add(1)
	return c.ReadCloser.Close()
}

type countingTransport struct {
	inner  http.RoundTripper
	closes *atomic.Int32
}

func (t countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(r)
	if resp != nil {
		resp.Body = closeCounter{ReadCloser: resp.Body, closes: t.closes}
	}
	return resp, err
}

func TestTurnClosesTheBody(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sse(w, `{"choices":[{"delta":{"content":"jeden"}}]}`)
		sse(w, `{"choices":[{"delta":{"content":"dwa"}}]}`)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	for _, tc := range []struct {
		name       string
		breakEarly bool
	}{
		{name: "drained to the end"},
		{name: "abandoned after one delta", breakEarly: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var closes atomic.Int32
			client := &http.Client{Transport: countingTransport{inner: server.Client().Transport, closes: &closes}}
			chat, _ := NewChat(openAIBinding(server.URL, "k"), client)
			stream, err := chat.Turn(context.Background(), ChatRequest{
				Lines: []Line{{From: LineFromPerson, Text: "hej"}},
			})
			if err != nil {
				t.Fatalf("Turn: %v", err)
			}
			for range stream {
				if tc.breakEarly {
					break
				}
			}
			if got := closes.Load(); got != 1 {
				t.Errorf("body closed %d times, want once", got)
			}
		})
	}
}

func TestChatMessages(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		req  ChatRequest
		want []chatMessage
	}{
		{
			name: "the brief carries the word budget",
			req:  ChatRequest{Brief: "You are Ania.", MaxWords: 40, Lines: []Line{{From: LineFromPerson, Text: "cześć"}}},
			want: []chatMessage{
				{Role: "system", Content: "You are Ania.\n\nAnswer in at most 40 words."},
				{Role: "user", Content: "cześć"},
			},
		},
		{
			name: "a budget with no brief still gets said",
			req:  ChatRequest{MaxWords: 12, Lines: []Line{{From: LineFromPerson, Text: "hej"}}},
			want: []chatMessage{
				{Role: "system", Content: "Answer in at most 12 words."},
				{Role: "user", Content: "hej"},
			},
		},
		{
			name: "her own lines come back as the assistant",
			req: ChatRequest{Lines: []Line{
				{From: LineFromPerson, Text: "sprawdź opony"},
				{From: LineFromStaff, Text: "już patrzę"},
				{From: LineFromPerson, Text: "i felgi"},
			}},
			want: []chatMessage{
				{Role: "user", Content: "sprawdź opony"},
				{Role: "assistant", Content: "już patrzę"},
				{Role: "user", Content: "i felgi"},
			},
		},
		{
			name: "an empty line is dropped, not sent",
			req:  ChatRequest{Lines: []Line{{From: LineFromPerson, Text: "  \n "}, {From: LineFromPerson, Text: "hej"}}},
			want: []chatMessage{{Role: "user", Content: "hej"}},
		},
		{
			name: "nothing to say renders nothing",
			req:  ChatRequest{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := chatMessages(tc.req)
			if len(got) != len(tc.want) {
				t.Fatalf("messages = %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("message %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestSSEData(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{
			name: "framing is not content",
			raw:  ": comment\nevent: ping\n\ndata: one\n\ndata: two\n\ndata: [DONE]\n\ndata: after\n",
			want: []string{"one", "two"},
		},
		{
			name: "CRLF is the same stream",
			raw:  "data: one\r\n\r\ndata: two\r\n\r\n",
			want: []string{"one", "two"},
		},
		{
			name: "an empty data line carries nothing",
			raw:  "data: \n\ndata: one\n\n",
			want: []string{"one"},
		},
		{
			name: "a clean end without the terminator is a finished reply",
			raw:  "data: one\n\n",
			want: []string{"one"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got []string
			for payload, err := range sseData(strings.NewReader(tc.raw)) {
				if err != nil {
					t.Fatalf("sseData: %v", err)
				}
				got = append(got, string(payload))
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("payloads = %v, want %v", got, tc.want)
			}
		})
	}
}

// failingReader is a connection that dies mid-stream.
type failingReader struct{ read bool }

func (f *failingReader) Read(p []byte) (int, error) {
	if f.read {
		return 0, errors.New("connection reset")
	}
	f.read = true
	return copy(p, "data: one\n\n"), nil
}

func TestSSEDataReportsABrokenStream(t *testing.T) {
	t.Parallel()
	var got []string
	var streamErr error
	for payload, err := range sseData(&failingReader{}) {
		if err != nil {
			streamErr = err
			continue
		}
		got = append(got, string(payload))
	}
	if len(got) != 1 || got[0] != "one" {
		t.Errorf("payloads = %v, want the one that arrived", got)
	}
	if streamErr == nil || !strings.Contains(streamErr.Error(), "connection reset") {
		t.Fatalf("error = %v, want the read failure", streamErr)
	}
	if !Retryable(streamErr) {
		t.Error("an unclassified transport failure is exactly what a retry is for")
	}
}

func TestSSEDataStopsWhenTheCallerDoes(t *testing.T) {
	t.Parallel()
	count := 0
	for range sseData(strings.NewReader("data: one\n\ndata: two\n\n")) {
		count++
		break
	}
	if count != 1 {
		t.Errorf("yielded %d payloads after a break, want 1", count)
	}
}

func TestRetryable(t *testing.T) {
	t.Parallel()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "no error"},
		{name: "a finished context", err: cancelled.Err()},
		{name: "a deadline that passed", err: context.DeadlineExceeded},
		{name: "our own request", err: fmt.Errorf("%w: no lines", ErrRequest)},
		{name: "a dialect we do not speak", err: fmt.Errorf("%w: mystery", ErrDialect)},
		{name: "configuration", err: fmt.Errorf("%w: typo", ErrConfig)},
		{name: "a server failure", err: &ProviderError{Code: http.StatusInternalServerError}, want: true},
		{name: "a timeout the provider reported", err: &ProviderError{Code: http.StatusRequestTimeout}, want: true},
		{name: "a refusal", err: &ProviderError{Code: http.StatusForbidden}},
		{name: "wrapped, still classified", err: fmt.Errorf("turn: %w", &ProviderError{Code: http.StatusServiceUnavailable}), want: true},
		{name: "anything else", err: errors.New("dial tcp: no route to host"), want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Retryable(tc.err); got != tc.want {
				t.Errorf("Retryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestEndpointHonoursAnAPIPath(t *testing.T) {
	t.Parallel()
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	binding := openAIBinding(server.URL, "k")
	binding.Provider.APIPath = "/v1/openai/chat"
	chat, _ := NewChat(binding, server.Client())
	stream, err := chat.Turn(context.Background(), ChatRequest{Lines: []Line{{From: LineFromPerson, Text: "hej"}}})
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	for range stream {
	}
	if gotPath != "/v1/openai/chat" {
		t.Errorf("path = %q, want the override", gotPath)
	}
}

func TestTurnSendsNoHeaderWithoutAKey(t *testing.T) {
	t.Parallel()
	seen := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, seen = r.Header["Authorization"]
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	chat, _ := NewChat(openAIBinding(server.URL, ""), server.Client())
	stream, err := chat.Turn(context.Background(), ChatRequest{Lines: []Line{{From: LineFromPerson, Text: "hej"}}})
	if err != nil {
		t.Fatalf("Turn: %v", err)
	}
	for range stream {
	}
	if seen {
		// An open local endpoint refuses `Bearer ` with nothing after it.
		t.Error("an empty key must send no Authorization header at all")
	}
}

func TestNewHTTPClientLeavesTheStreamOpen(t *testing.T) {
	t.Parallel()
	if client := NewHTTPClient(); client.Timeout != 0 {
		t.Errorf("Timeout = %v, want none: a whole-request deadline cuts a long reply mid-sentence", client.Timeout)
	}
}
