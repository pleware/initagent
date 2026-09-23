package hub

import "errors"

// BuildBoxManifest assembles the config manifest one box syncs down (58):
// the box row, its bound organizations, the staff roster as each bound
// organization sees it, the box's narrator, and the resolved model roster.
// Only bound organizations appear — an org elsewhere on the installation
// stays out of the manifest. The exact shape:
//
//	{
//	  "version": <box.config_version>,
//	  "box":     {"id","slug","name","edition","hostId"},
//	  "orgs":    [{"id","name","level"} for each bound org],
//	  "staff":   {"<orgId>": StaffForOrg(org) for each bound org},
//	  "narrator": <first StaffForBox row, or null when unseeded>,
//	  "models":  {"<purpose>": {"id","source","quant","file","digest","licence","files"}}
//	}
//
// The models section is the resolved roster: per purpose the box's
// override wins when present, the factory assignment answers when not, and
// a purpose with neither is omitted. `file` is the anchor artifact a
// file-consuming program is pointed at; `files` is every artifact the pin is
// made of, which is what a directory-consuming program (the ear on a
// faster-whisper pin) needs and what the box's puller fetches — always an
// array, empty for a single-artifact pin. A missing box is an error; the sync
// handler checks existence first and answers 404 itself.
func (s *Store) BuildBoxManifest(boxID string) (map[string]any, error) {
	box, err := s.GetBox(boxID)
	if err != nil {
		return nil, err
	}
	if box == nil {
		return nil, errors.New("no such box")
	}
	orgIDs, err := s.ListBoxOrgs(boxID)
	if err != nil {
		return nil, err
	}
	orgs := []map[string]any{}
	staff := map[string]any{}
	for _, orgID := range orgIDs {
		org, err := s.OrgById(orgID)
		if err != nil {
			return nil, err
		}
		if org == nil {
			// A binding whose org row is gone is stale data, not a reason
			// to fail the sync: skip it in both lists.
			continue
		}
		orgs = append(orgs, map[string]any{"id": org.Id, "name": org.Name, "level": org.Level})
		roster, err := s.StaffForOrg(orgID)
		if err != nil {
			return nil, err
		}
		staff[orgID] = roster
	}
	var narrator any
	roster, err := s.StaffForBox(boxID)
	if err != nil {
		return nil, err
	}
	if len(roster) > 0 {
		narrator = roster[0]
	}
	resolved, err := s.ResolvedModels(boxID)
	if err != nil {
		return nil, err
	}
	models := map[string]map[string]any{}
	for _, purpose := range modelPurposeOrder {
		m, ok := resolved[purpose]
		if !ok {
			continue
		}
		models[purpose] = map[string]any{
			"id":      m.ID,
			"source":  m.Source,
			"engine":  m.Engine,
			"quant":   m.Quant,
			"file":    m.File,
			"digest":  m.Digest,
			"licence": m.Licence,
			"files":   manifestFiles(m.Files, m.File, m.Digest),
		}
	}
	return map[string]any{
		"version": box.ConfigVersion,
		"box": map[string]any{
			"id":      box.ID,
			"slug":    box.Slug,
			"name":    box.Name,
			"edition": box.Edition,
			"hostId":  box.HostID,
		},
		"orgs":     orgs,
		"staff":    staff,
		"narrator": narrator,
		"models":   models,
	}, nil
}

// manifestFiles is the models-section spelling of a pin's file list: an
// always-present array, so a box's parser reads one shape whether the model is
// a single artifact or a directory.
//
// The anchor's entry carries the pin's own digest when the list names none of
// its own. The seed writes file NAMES only and `digest` stays the anchor's
// verified digest, so without this the one file the hub has already verified
// would be the one file the box pulls unverified.
func manifestFiles(files []ModelFile, anchor, anchorDigest string) []ModelFile {
	if files == nil {
		return []ModelFile{}
	}
	out := make([]ModelFile, len(files))
	copy(out, files)
	for i := range out {
		if out[i].Digest == "" && out[i].File == anchor && anchorDigest != "" {
			out[i].Digest = anchorDigest
		}
	}
	return out
}
