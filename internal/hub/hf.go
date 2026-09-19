package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pleware/initagent/internal/authz"
)

// hfSearcher is the thin seam over the Hugging Face API. The Server depends
// on this interface, never on the HTTP implementation, so endpoint tests
// inject a mock and httpHFSearcher below is the only thing that touches
// huggingface.co.
type hfSearcher interface {
	// Search answers one page of the HF model catalog for a query, sorted
	// by downloads. pipelineTag narrows the page to one HF pipeline tag
	// (empty = no filter).
	Search(ctx context.Context, query, pipelineTag string, limit int) ([]hfModel, error)
	// RepoFiles answers the .gguf files of one repo, each with the
	// canonical quant parsed from its filename.
	RepoFiles(ctx context.Context, repo string) ([]hfFile, error)
}

// hfModel is one parsed catalog entry. ID is the HF id (org/repo); Org and
// Name are its two halves. Gated reports whether the repo requires
// accepting a license or gate before download — metadata the admin sees,
// not something the hub acts on.
type hfModel struct {
	ID          string
	Org         string
	Name        string
	PipelineTag string
	LibraryName string
	Licence     string
	Downloads   int64
	Gated       bool
}

// hfFile is one .gguf file of a repo, with the canonical quant parsed from
// its filename.
type hfFile struct {
	Path  string
	Quant string
}

// hfSearchResult is the wire shape of one search hit: identity, the model
// card's pipeline tag and library, the licence from tags, and the purpose
// the UI pre-fills — a suggestion the admin can override.
type hfSearchResult struct {
	Org              string `json:"org"`
	Name             string `json:"name"`
	ID               string `json:"id"`
	PipelineTag      string `json:"pipelineTag"`
	LibraryName      string `json:"libraryName"`
	Licence          string `json:"licence"`
	Downloads        int64  `json:"downloads"`
	SuggestedPurpose string `json:"suggestedPurpose"`
}

// hfRepoFile is the wire shape of one repo file: the filename and its
// canonical quant.
type hfRepoFile struct {
	Filename string `json:"filename"`
	Quant    string `json:"quant"`
}

const (
	hfDefaultLimit = 20
	hfMaxLimit     = 50
	hfTimeout      = 15 * time.Second
)

// suggestPurpose maps an HF pipeline tag to the hub purpose the admin will
// most likely want. It is a suggestion, not an automatic decision: the
// admin can change it on the form. Tags with no sensible mapping suggest
// nothing.
func suggestPurpose(pipelineTag string) string {
	switch pipelineTag {
	case "text-generation", "image-text-to-text", "text2text-generation", "conversational":
		return "persona"
	case "sentence-similarity", "feature-extraction":
		return "embedding"
	case "automatic-speech-recognition":
		return "stt"
	case "audio-classification":
		return "vad"
	case "text-to-speech":
		return "tts"
	default:
		return ""
	}
}

// ggufQuantRe matches the {name}-{QUANT}.gguf filename convention: the
// quant is the last dash-separated token before the .gguf extension.
var ggufQuantRe = regexp.MustCompile(`-([A-Za-z0-9_]+)\.gguf$`)

// parseGGUFQuant extracts the quant from a GGUF filename and validates it
// against the canonical set via ParseQuant. The match is case-insensitive
// and the answer is canonical uppercase. A filename outside the convention
// or a non-canonical quant is ("", false) — never an error, because a repo
// tree carries files that are simply not GGUF.
func parseGGUFQuant(filename string) (string, bool) {
	m := ggufQuantRe.FindStringSubmatch(filename)
	if m == nil {
		return "", false
	}
	quant, err := ParseQuant(m[1])
	if err != nil {
		return "", false
	}
	return quant, true
}

// licenseFromTags reads the licence off a model card's tags: the first tag
// carrying the license: prefix, prefix stripped. Cards without the tag
// answer "".
func licenseFromTags(tags []string) string {
	for _, tag := range tags {
		if _, ok := strings.CutPrefix(strings.ToLower(tag), "license:"); ok {
			return strings.TrimSpace(tag[len("license:"):])
		}
	}
	return ""
}

// httpHFSearcher is the real HF client: two GETs against huggingface.co/api
// with a 15s timeout, no token (prototype). The base URL is a field so
// tests can point it at an httptest server.
type httpHFSearcher struct {
	client *http.Client
	base   string
}

func newHFSearcher() *httpHFSearcher {
	return &httpHFSearcher{
		client: &http.Client{Timeout: hfTimeout},
		base:   "https://huggingface.co/api",
	}
}

// hfSearchHit is the raw shape of one entry in the HF search response.
// Gated arrives as a bool or a string ("auto"/"manual"), so it stays any
// and is normalized on parse.
type hfSearchHit struct {
	ID          string   `json:"id"`
	PipelineTag string   `json:"pipeline_tag"`
	LibraryName string   `json:"library_name"`
	Tags        []string `json:"tags"`
	Downloads   int64    `json:"downloads"`
	Gated       any      `json:"gated"`
}

// hfTreeHit is the raw shape of one entry in the HF tree response.
type hfTreeHit struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// Search queries the HF catalog. Entries whose id carries no org/repo
// split are malformed and skipped rather than surfaced half-parsed. A
// non-empty pipelineTag rides along as HF's pipeline_tag filter.
func (h *httpHFSearcher) Search(ctx context.Context, query, pipelineTag string, limit int) ([]hfModel, error) {
	u := h.base + "/models?search=" + url.QueryEscape(query) +
		"&limit=" + strconv.Itoa(limit) + "&sort=downloads&full=true"
	if pipelineTag != "" {
		u += "&pipeline_tag=" + url.QueryEscape(pipelineTag)
	}
	var hits []hfSearchHit
	if err := h.get(ctx, u, &hits); err != nil {
		return nil, fmt.Errorf("huggingface search: %w", err)
	}
	out := []hfModel{}
	for _, hit := range hits {
		org, name, ok := strings.Cut(hit.ID, "/")
		if !ok || org == "" || name == "" {
			continue
		}
		out = append(out, hfModel{
			ID:          hit.ID,
			Org:         org,
			Name:        name,
			PipelineTag: hit.PipelineTag,
			LibraryName: hit.LibraryName,
			Licence:     licenseFromTags(hit.Tags),
			Downloads:   hit.Downloads,
			Gated:       hfGated(hit.Gated),
		})
	}
	return out, nil
}

// RepoFiles answers the canonical GGUF files of one repo. Non-file entries
// and files whose quant does not parse are skipped: the admin can only pin
// what the registry would accept.
func (h *httpHFSearcher) RepoFiles(ctx context.Context, repo string) ([]hfFile, error) {
	u := h.base + "/models/" + repoPath(repo) + "/tree/main"
	var hits []hfTreeHit
	if err := h.get(ctx, u, &hits); err != nil {
		return nil, fmt.Errorf("huggingface tree: %w", err)
	}
	out := []hfFile{}
	for _, hit := range hits {
		if hit.Type != "file" {
			continue
		}
		quant, ok := parseGGUFQuant(hit.Path)
		if !ok {
			continue
		}
		out = append(out, hfFile{Path: hit.Path, Quant: quant})
	}
	return out, nil
}

// hfGated normalizes the gated field, which HF serves as a bool or a
// string ("auto"/"manual").
func hfGated(v any) bool {
	switch v := v.(type) {
	case bool:
		return v
	case string:
		return v != "" && v != "false"
	default:
		return false
	}
}

// repoPath escapes each segment of an org/repo id for the URL path while
// keeping the slash between them.
func repoPath(repo string) string {
	org, name, ok := strings.Cut(repo, "/")
	if !ok {
		return url.PathEscape(repo)
	}
	return url.PathEscape(org) + "/" + url.PathEscape(name)
}

// get fetches and decodes one JSON response. A non-2xx status is an error
// carrying the status and a bounded slice of the body; a truncated body is
// the decoder's error.
func (h *httpHFSearcher) get(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// handleHfSearch proxies a catalog search to Hugging Face for the admin
// models page (results grouped by org in the UI). The optional pipelineTag
// narrows the page to one HF pipeline tag — empty means no filter — and the
// limit is optional (default 20, capped at 50); a missing q is a 400; an
// HF failure is a 502 — the hub answered, but the upstream did not.
func (s *Server) handleHfSearch(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpError(w, http.StatusBadRequest, "q is required")
		return
	}
	limit := hfDefaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			httpError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(n, hfMaxLimit)
	}
	models, err := s.hf.Search(r.Context(), q, r.URL.Query().Get("pipelineTag"), limit)
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	out := []hfSearchResult{}
	for _, m := range models {
		out = append(out, hfSearchResult{
			Org:              m.Org,
			Name:             m.Name,
			ID:               m.ID,
			PipelineTag:      m.PipelineTag,
			LibraryName:      m.LibraryName,
			Licence:          m.Licence,
			Downloads:        m.Downloads,
			SuggestedPurpose: suggestPurpose(m.PipelineTag),
		})
	}
	writeJSON(w, out)
}

// handleHfRepoFiles answers the .gguf files of one HF repo — each with the
// canonical quant parsed from its filename — so the admin can pick the
// quant to pin. Only canonical GGUF files appear; everything else in the
// tree is skipped. An HF failure is a 502.
func (s *Server) handleHfRepoFiles(w http.ResponseWriter, r *http.Request, cred authz.Credential) {
	if !cred.Can(authz.AdminModels, "", "") {
		forbid(w, authz.ErrForbidden)
		return
	}
	files, err := s.hf.RepoFiles(r.Context(), r.PathValue("org")+"/"+r.PathValue("repo"))
	if err != nil {
		httpError(w, http.StatusBadGateway, err.Error())
		return
	}
	out := []hfRepoFile{}
	for _, f := range files {
		out = append(out, hfRepoFile{Filename: f.Path, Quant: f.Quant})
	}
	writeJSON(w, out)
}
