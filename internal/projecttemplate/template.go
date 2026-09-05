// Package projecttemplate is the create-project catalogue (drafts 26, 38).
//
// A template is a human label over fields the project already stores: the
// delivery contract, whether a repo is required, and which capability.Task
// values a worker must declare. It is not a project.kind column. Milestone 0
// ships one live template (software); the others appear as coming soon.
package projecttemplate

import (
	"slices"

	"github.com/pleware/initagent/internal/registry/ai/capability"
)

// ID is a stable catalogue key stored on the project row. It is not shown
// as a HuggingFace slug and it is not a kind enum.
type ID string

const (
	Software  ID = "software"
	Website   ID = "website"
	Video     ID = "video"
	Song      ID = "song"
	Poem      ID = "poem"
	Audiobook ID = "audiobook"
)

// Template is one shipped create-project choice.
type Template struct {
	ID            ID                  `json:"id"`
	Label         string              `json:"label"`
	Live          bool                `json:"live"`
	Contract      capability.Contract `json:"contract"`
	NeedsRepo     bool                `json:"needsRepo"`
	RequiredTasks []capability.Task   `json:"requiredTasks"`
}

var catalogue = []Template{
	{
		ID:            Software,
		Label:         "Computer program",
		Live:          true,
		Contract:      capability.ContractChange,
		NeedsRepo:     true,
		RequiredTasks: []capability.Task{capability.TextGeneration},
	},
	{
		ID:        Website,
		Label:     "Website",
		Live:      false,
		Contract:  capability.ContractChange,
		NeedsRepo: true,
		RequiredTasks: []capability.Task{
			capability.TextGeneration,
		},
	},
	{
		ID:        Video,
		Label:     "Video",
		Live:      false,
		Contract:  capability.ContractArtifact,
		NeedsRepo: false,
	},
	{
		ID:            Song,
		Label:         "Song",
		Live:          false,
		Contract:      capability.ContractArtifact,
		NeedsRepo:     false,
		RequiredTasks: []capability.Task{capability.TextToAudio},
	},
	{
		ID:            Poem,
		Label:         "Poem",
		Live:          false,
		Contract:      capability.ContractArtifact,
		NeedsRepo:     false,
		RequiredTasks: []capability.Task{capability.TextGeneration},
	},
	{
		ID:            Audiobook,
		Label:         "Audiobook",
		Live:          false,
		Contract:      capability.ContractArtifact,
		NeedsRepo:     false,
		RequiredTasks: []capability.Task{capability.TextToAudio},
	},
}

// Catalogue returns every shipped template in picker order.
func Catalogue() []Template {
	return slices.Clone(catalogue)
}

// Lookup finds a template by its catalogue id.
func Lookup(id string) (Template, bool) {
	i := slices.IndexFunc(catalogue, func(t Template) bool {
		return string(t.ID) == id
	})
	if i < 0 {
		return Template{}, false
	}
	return catalogue[i], true
}

// Live reports whether id is a template that create-project may use.
func Live(id string) bool {
	t, ok := Lookup(id)
	return ok && t.Live
}
