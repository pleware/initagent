package hub

import "errors"

// BuildBoxManifest assembles the config manifest one box syncs down (58):
// the box row, its bound organizations, the staff roster as each bound
// organization sees it, and the box's narrator. Only bound organizations
// appear — an org elsewhere on the installation stays out of the manifest.
// The exact shape:
//
//	{
//	  "version": <box.config_version>,
//	  "box":     {"id","slug","name","edition","hostId"},
//	  "orgs":    [{"id","name"} for each bound org],
//	  "staff":   {"<orgId>": StaffForOrg(org) for each bound org},
//	  "narrator": <first StaffForBox row, or null when unseeded>
//	}
//
// A missing box is an error; the sync handler checks existence first and
// answers 404 itself.
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
		orgs = append(orgs, map[string]any{"id": org.Id, "name": org.Name})
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
	}, nil
}
