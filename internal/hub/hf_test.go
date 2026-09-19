package hub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pleware/initagent/internal/offering"
)

// mockHFSearcher is an hfSearcher with scripted answers, so endpoint tests
// never touch the network.
type mockHFSearcher struct {
	search    func(ctx context.Context, query string, limit int) ([]hfModel, error)
	repoFiles func(ctx context.Context, repo string) ([]hfFile, error)
}

func (m mockHFSearcher) Search(ctx context.Context, query string, limit int) ([]hfModel, error) {
	return m.search(ctx, query, limit)
}

func (m mockHFSearcher) RepoFiles(ctx context.Context, repo string) ([]hfFile, error) {
	return m.repoFiles(ctx, repo)
}

func TestSuggestPurpose(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want string
	}{
		{"text generation", "text-generation", "persona"},
		{"image text to text", "image-text-to-text", "persona"},
		{"text2text", "text2text-generation", "persona"},
		{"conversational", "conversational", "persona"},
		{"sentence similarity", "sentence-similarity", "embedding"},
		{"feature extraction", "feature-extraction", "embedding"},
		{"asr", "automatic-speech-recognition", "stt"},
		{"text to speech is skipped", "text-to-speech", ""},
		{"unmapped tag", "image-classification", ""},
		{"empty tag", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := suggestPurpose(tt.tag); got != tt.want {
				t.Errorf("suggestPurpose(%q) = %q, want %q", tt.tag, got, tt.want)
			}
		})
	}
}

func TestParseGGUFQuant(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     string
		ok       bool
	}{
		{"canonical k-quant", "Qwen3.5-4B-Q4_K_M.gguf", "Q4_K_M", true},
		{"lowercase normalizes", "model-q4_k_m.gguf", "Q4_K_M", true},
		{"i-quant", "model-IQ2_XXS.gguf", "IQ2_XXS", true},
		{"float quant", "model-BF16.gguf", "BF16", true},
		{"nested path", "sub/dir/model-Q5_K_S.gguf", "Q5_K_S", true},
		{"non-canonical quant", "model-Q4_K.gguf", "", false},
		{"made-up quant", "model-bogus.gguf", "", false},
		{"no dash before extension", "Q4_K_M.gguf", "", false},
		{"not a gguf file", "model.safetensors", "", false},
		{"readme", "README.md", "", false},
		{"empty quant token", "model-.gguf", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseGGUFQuant(tt.filename)
			if ok != tt.ok || got != tt.want {
				t.Errorf("parseGGUFQuant(%q) = (%q, %v), want (%q, %v)", tt.filename, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestLicenseFromTags(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want string
	}{
		{"first license tag", []string{"gguf", "license:apache-2.0", "license:mit"}, "apache-2.0"},
		{"only tag", []string{"license:mit"}, "mit"},
		{"no license tag", []string{"gguf", "text-generation"}, ""},
		{"no tags", []string{}, ""},
		{"mixed case prefix", []string{"License:MIT"}, "MIT"},
		{"empty licence", []string{"license:"}, ""},
		{"whitespace trimmed", []string{"license: apache-2.0 "}, "apache-2.0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := licenseFromTags(tt.tags); got != tt.want {
				t.Errorf("licenseFromTags(%v) = %q, want %q", tt.tags, got, tt.want)
			}
		})
	}
}

func TestHfSearchEndpoint(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	gotQuery := ""
	gotLimit := 0
	f.srv.hf = mockHFSearcher{
		search: func(ctx context.Context, query string, limit int) ([]hfModel, error) {
			gotQuery, gotLimit = query, limit
			return []hfModel{
				{
					ID:          "unsloth/Qwen3.5-4B-GGUF",
					Org:         "unsloth",
					Name:        "Qwen3.5-4B-GGUF",
					PipelineTag: "text-generation",
					LibraryName: "gguf",
					Licence:     "apache-2.0",
					Downloads:   42,
				},
			}, nil
		},
		repoFiles: func(ctx context.Context, repo string) ([]hfFile, error) {
			return nil, errors.New("unexpected repo call")
		},
	}

	resp := f.do(t, http.MethodGet, "/api/admin/models/hf/search?q=llama&limit=5", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: %d, want 200", resp.StatusCode)
	}
	if gotQuery != "llama" || gotLimit != 5 {
		t.Errorf("searcher got (%q, %d), want (llama, 5)", gotQuery, gotLimit)
	}
	var results []hfSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %+v, want one hit", results)
	}
	got := results[0]
	if got.Org != "unsloth" || got.Name != "Qwen3.5-4B-GGUF" || got.ID != "unsloth/Qwen3.5-4B-GGUF" ||
		got.PipelineTag != "text-generation" || got.LibraryName != "gguf" ||
		got.Licence != "apache-2.0" || got.Downloads != 42 || got.SuggestedPurpose != "persona" {
		t.Errorf("result = %+v, want the submitted hit with a persona suggestion", got)
	}

	// No limit: the default 20 reaches the searcher.
	resp = f.do(t, http.MethodGet, "/api/admin/models/hf/search?q=llama", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search without limit: %d, want 200", resp.StatusCode)
	}
	if gotLimit != 20 {
		t.Errorf("searcher got limit %d, want the default 20", gotLimit)
	}

	// An oversized limit caps at 50.
	resp = f.do(t, http.MethodGet, "/api/admin/models/hf/search?q=llama&limit=500", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search with huge limit: %d, want 200", resp.StatusCode)
	}
	if gotLimit != 50 {
		t.Errorf("searcher got limit %d, want the capped 50", gotLimit)
	}
}

func TestHfSearchRefusals(t *testing.T) {
	f := claimedHub(t, offering.Hosted)

	// An empty q is a 400 and never reaches the searcher.
	f.srv.hf = mockHFSearcher{
		search: func(ctx context.Context, query string, limit int) ([]hfModel, error) {
			t.Fatalf("searcher called for a malformed query %q", query)
			return nil, nil
		},
		repoFiles: func(ctx context.Context, repo string) ([]hfFile, error) {
			return nil, errors.New("unexpected repo call")
		},
	}
	for _, path := range []string{
		"/api/admin/models/hf/search",
		"/api/admin/models/hf/search?limit=5",
		"/api/admin/models/hf/search?q=%20%20",
	} {
		resp := f.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", path, resp.StatusCode)
		}
	}

	// A bogus limit is a 400.
	resp := f.do(t, http.MethodGet, "/api/admin/models/hf/search?q=llama&limit=abc", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("bad limit: %d, want 400", resp.StatusCode)
	}

	// An HF failure is a 502.
	f.srv.hf = mockHFSearcher{
		search: func(ctx context.Context, query string, limit int) ([]hfModel, error) {
			return nil, errors.New("hf down")
		},
		repoFiles: func(ctx context.Context, repo string) ([]hfFile, error) {
			return nil, errors.New("unexpected repo call")
		},
	}
	resp = f.do(t, http.MethodGet, "/api/admin/models/hf/search?q=llama", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("hf failure: %d, want 502", resp.StatusCode)
	}
}

func TestHfRepoFilesEndpoint(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	gotRepo := ""
	f.srv.hf = mockHFSearcher{
		search: func(ctx context.Context, query string, limit int) ([]hfModel, error) {
			return nil, errors.New("unexpected search call")
		},
		repoFiles: func(ctx context.Context, repo string) ([]hfFile, error) {
			gotRepo = repo
			return []hfFile{
				{Path: "Qwen3.5-4B-Q4_K_M.gguf", Quant: "Q4_K_M"},
				{Path: "Qwen3.5-4B-IQ2_XXS.gguf", Quant: "IQ2_XXS"},
			}, nil
		},
	}

	resp := f.do(t, http.MethodGet, "/api/admin/models/hf/repo/unsloth/Qwen3.5-4B-GGUF", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("repo files: %d, want 200", resp.StatusCode)
	}
	if gotRepo != "unsloth/Qwen3.5-4B-GGUF" {
		t.Errorf("searcher got repo %q, want unsloth/Qwen3.5-4B-GGUF", gotRepo)
	}
	var files []hfRepoFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %+v, want two gguf entries", files)
	}
	if files[0].Filename != "Qwen3.5-4B-Q4_K_M.gguf" || files[0].Quant != "Q4_K_M" ||
		files[1].Filename != "Qwen3.5-4B-IQ2_XXS.gguf" || files[1].Quant != "IQ2_XXS" {
		t.Errorf("files = %+v, want the submitted entries", files)
	}
}

func TestHfRepoFilesFailure(t *testing.T) {
	f := claimedHub(t, offering.Hosted)
	f.srv.hf = mockHFSearcher{
		search: func(ctx context.Context, query string, limit int) ([]hfModel, error) {
			return nil, errors.New("unexpected search call")
		},
		repoFiles: func(ctx context.Context, repo string) ([]hfFile, error) {
			return nil, errors.New("hf down")
		},
	}
	resp := f.do(t, http.MethodGet, "/api/admin/models/hf/repo/unsloth/repo", nil)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("hf failure: %d, want 502", resp.StatusCode)
	}
}

func TestHfRoutesRefuseNonAdmin(t *testing.T) {
	f := hostedCustomer(t)
	for _, path := range []string{
		"/api/admin/models/hf/search?q=llama",
		"/api/admin/models/hf/repo/unsloth/Qwen3.5-4B-GGUF",
	} {
		resp := f.do(t, http.MethodGet, path, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s: %d, want 403", path, resp.StatusCode)
		}
	}
}

func TestHfRoutesRequireAuth(t *testing.T) {
	srv := newHub(t, t.TempDir(), offering.Selfhost)
	ts := httptest.NewServer(srv.mux)
	t.Cleanup(ts.Close)

	resp := requestJSON(t, ts, &http.Client{}, http.MethodGet, "/api/admin/models/hf/search?q=llama", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bare search: %d, want 401", resp.StatusCode)
	}
}

func TestHTTPSearcherSearch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("search") != "llama" || r.URL.Query().Get("limit") != "5" ||
			r.URL.Query().Get("sort") != "downloads" || r.URL.Query().Get("full") != "true" {
			t.Errorf("bad search query: %s", r.URL.RawQuery)
		}
		writeJSON(w, []hfSearchHit{
			{
				ID:          "meta-llama/Llama-3.1-8B-Instruct-GGUF",
				PipelineTag: "text-generation",
				LibraryName: "gguf",
				Tags:        []string{"gguf", "license:llama3.1"},
				Downloads:   100,
				Gated:       "auto",
			},
			{
				ID:        "no-namespace",
				Downloads: 1,
				Gated:     false,
			},
		})
	}))
	t.Cleanup(ts.Close)

	h := &httpHFSearcher{client: ts.Client(), base: ts.URL}
	models, err := h.Search(context.Background(), "llama", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("Search = %+v, want one parsed hit (the id without a namespace skipped)", models)
	}
	m := models[0]
	if m.ID != "meta-llama/Llama-3.1-8B-Instruct-GGUF" || m.Org != "meta-llama" ||
		m.Name != "Llama-3.1-8B-Instruct-GGUF" || m.PipelineTag != "text-generation" ||
		m.LibraryName != "gguf" || m.Licence != "llama3.1" || m.Downloads != 100 || !m.Gated {
		t.Errorf("parsed = %+v, want the submitted entry", m)
	}
}

func TestHTTPSearcherRepoFiles(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/unsloth/Qwen3.5-4B-GGUF/tree/main" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, []hfTreeHit{
			{Type: "file", Path: "Qwen3.5-4B-Q4_K_M.gguf"},
			{Type: "file", Path: "Qwen3.5-4B-q5_k_m.gguf"},
			{Type: "file", Path: "Qwen3.5-4B-bogus.gguf"},
			{Type: "file", Path: "config.json"},
			{Type: "directory", Path: "subdir"},
		})
	}))
	t.Cleanup(ts.Close)

	h := &httpHFSearcher{client: ts.Client(), base: ts.URL}
	files, err := h.RepoFiles(context.Background(), "unsloth/Qwen3.5-4B-GGUF")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("RepoFiles = %+v, want the two canonical gguf files", files)
	}
	if files[0].Path != "Qwen3.5-4B-Q4_K_M.gguf" || files[0].Quant != "Q4_K_M" ||
		files[1].Path != "Qwen3.5-4B-q5_k_m.gguf" || files[1].Quant != "Q5_K_M" {
		t.Errorf("RepoFiles = %+v, want canonical quants only", files)
	}
}

func TestHTTPSearcherFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(ts.Close)

	h := &httpHFSearcher{client: ts.Client(), base: ts.URL}
	if _, err := h.Search(context.Background(), "llama", 5); err == nil {
		t.Error("Search on a 500 answered nil error, want an error")
	}
	if _, err := h.RepoFiles(context.Background(), "org/repo"); err == nil {
		t.Error("RepoFiles on a 500 answered nil error, want an error")
	}
}
