package hub

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
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
	// Inspect answers the derived metadata for one pinned artifact — the HF
	// catalog entry (pipeline tag, library, base model, downloads, gate) and,
	// for a GGUF file, the architecture and context length read from the file
	// header over a ranged GET, never the whole file.
	Inspect(ctx context.Context, repo, rev, file string, readGGUF bool) (ModelMeta, error)
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

// Inspect answers the derived metadata for one pinned artifact: the HF
// catalog entry (pipeline tag, library name, base model, downloads, gate)
// and, when readGGUF is set, the architecture and context length read from
// the GGUF header over a ranged GET. A header read failure is non-fatal —
// the catalog half still answers and the GGUF fields stay empty (unknown,
// not fabricated) — because a non-GGUF pin legitimately has no header.
func (h *httpHFSearcher) Inspect(ctx context.Context, repo, rev, file string, readGGUF bool) (ModelMeta, error) {
	var meta ModelMeta
	var hit struct {
		PipelineTag string   `json:"pipeline_tag"`
		LibraryName string   `json:"library_name"`
		Tags        []string `json:"tags"`
		Downloads   int64    `json:"downloads"`
		Gated       any      `json:"gated"`
	}
	if err := h.get(ctx, h.base+"/models/"+repoPath(repo), &hit); err != nil {
		return meta, fmt.Errorf("huggingface model: %w", err)
	}
	meta.PipelineTag = hit.PipelineTag
	meta.LibraryName = hit.LibraryName
	meta.BaseModel = baseModelFromTags(hit.Tags)
	meta.Downloads = hit.Downloads
	meta.Gated = hfGated(hit.Gated)
	if !readGGUF {
		return meta, nil
	}
	arch, ctxLen, err := h.ggufHeader(ctx, repo, rev, file)
	if err != nil {
		return meta, nil
	}
	meta.Architecture = arch
	meta.ContextLength = ctxLen
	return meta, nil
}

// baseModelFromTags reads the unquantized source model off a model card's
// tags: the plain base_model: tag wins, and a base_model:quantized: tag is
// the fallback. A card with neither answers "".
func baseModelFromTags(tags []string) string {
	var quantized string
	for _, t := range tags {
		if v, ok := strings.CutPrefix(t, "base_model:quantized:"); ok {
			quantized = v
			continue
		}
		if v, ok := strings.CutPrefix(t, "base_model:"); ok {
			return v
		}
	}
	return quantized
}

// ggufHeaderBytes is the ranged window read from the front of a GGUF file.
// The architecture and context length keys sit before the tokenizer vocab
// (the largest part of the header) — measured on the three pinned GGUF
// models the metadata ends by byte 755 — so 8 KB is a 10× margin that never
// touches the multi-GB tensor data. The parser stops at the first
// tokenizer.* key, so the window only has to cover up to context_length.
const ggufHeaderBytes = 8192

// ggufHeader reads the metadata block of one GGUF file over a ranged GET and
// parses general.architecture plus the <arch>.context_length key.
func (h *httpHFSearcher) ggufHeader(ctx context.Context, repo, rev, file string) (arch string, ctxLen int64, err error) {
	u := h.base + "/models/" + repoPath(repo) + "/resolve/" + url.PathEscape(rev) + "/" + url.PathEscape(file)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", ggufHeaderBytes-1))
	resp, err := h.client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	// 200 means the host ignored the Range; 206 is partial content.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", 0, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, ggufHeaderBytes))
	if err != nil {
		return "", 0, err
	}
	return parseGGUFHeader(data)
}

// parseGGUFHeader walks the GGUF metadata key/value pairs at the front of
// the file and answers general.architecture and the <arch>.context_length
// key. It stops at the first tokenizer.* key, so the tokenizer vocab — the
// largest part of the header — is never traversed. Values are little-endian.
func parseGGUFHeader(data []byte) (arch string, ctxLen int64, err error) {
	if len(data) < 24 || string(data[:4]) != "GGUF" {
		return "", 0, errors.New("not a GGUF file")
	}
	kvCount := binary.LittleEndian.Uint64(data[16:24])
	off := 24
	for i := uint64(0); i < kvCount; i++ {
		key, noff, err := ggufString(data, off)
		if err != nil {
			return arch, ctxLen, err
		}
		off = noff
		if off+4 > len(data) {
			return arch, ctxLen, errors.New("truncated GGUF header")
		}
		vt := binary.LittleEndian.Uint32(data[off : off+4])
		off += 4
		if strings.HasPrefix(key, "tokenizer.") {
			break // the vocab is the largest part of the header; never read it
		}
		switch vt {
		case 4: // uint32
			if off+4 > len(data) {
				return arch, ctxLen, errors.New("truncated GGUF header")
			}
			v := binary.LittleEndian.Uint32(data[off : off+4])
			off += 4
			if strings.Contains(key, "context_length") {
				ctxLen = int64(v)
			}
		case 8: // string
			v, noff, err := ggufString(data, off)
			if err != nil {
				return arch, ctxLen, err
			}
			off = noff
			if key == "general.architecture" {
				arch = v
			}
		case 10: // uint64
			if off+8 > len(data) {
				return arch, ctxLen, errors.New("truncated GGUF header")
			}
			v := binary.LittleEndian.Uint64(data[off : off+8])
			off += 8
			if strings.Contains(key, "context_length") {
				ctxLen = int64(v)
			}
		default:
			noff, err := ggufSkip(data, off, vt)
			if err != nil {
				return arch, ctxLen, err
			}
			off = noff
		}
	}
	return arch, ctxLen, nil
}

// ggufString reads a length-prefixed UTF-8 string (u64 length + bytes).
func ggufString(data []byte, off int) (string, int, error) {
	if off+8 > len(data) {
		return "", off, errors.New("truncated GGUF header")
	}
	n := binary.LittleEndian.Uint64(data[off : off+8])
	off += 8
	if off+int(n) > len(data) {
		return "", off, errors.New("truncated GGUF header")
	}
	return string(data[off : off+int(n)]), off + int(n), nil
}

// ggufTypeSizes maps a GGUF value type to its scalar width for array
// skipping (type 8, string, and type 9, array, are handled separately).
var ggufTypeSizes = [...]int{1, 1, 2, 2, 4, 4, 4, 1, 8, 4, 8, 8, 8}

// ggufSkip advances past one metadata value without materializing it, so the
// tokenizer vocab arrays never load. Type 9 (array) recurses over its
// element type; a string array skips each length-prefixed element.
func ggufSkip(data []byte, off int, vt uint32) (int, error) {
	if vt == 8 { // string
		_, noff, err := ggufString(data, off)
		return noff, err
	}
	if vt == 9 { // array
		if off+12 > len(data) {
			return off, errors.New("truncated GGUF header")
		}
		at := binary.LittleEndian.Uint32(data[off : off+4])
		alen := binary.LittleEndian.Uint64(data[off+4 : off+12])
		off += 12
		if at == 8 {
			for i := uint64(0); i < alen; i++ {
				_, noff, err := ggufString(data, off)
				if err != nil {
					return off, err
				}
				off = noff
			}
			return off, nil
		}
		w := 8
		if int(at) < len(ggufTypeSizes) {
			w = ggufTypeSizes[at]
		}
		return off + int(alen)*w, nil
	}
	w := 8
	if int(vt) < len(ggufTypeSizes) {
		w = ggufTypeSizes[vt]
	}
	return off + w, nil
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
