package capability

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// declaredTasks reads this package's own source for constants typed Task.
//
// The alternative is a hand-maintained list in the test, which drifts exactly
// when it matters: someone adds a Task constant, forgets the Registry entry,
// and the test that was supposed to catch it was never told about the new
// constant.
func declaredTasks(t *testing.T) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "capability.go", nil, 0)
	if err != nil {
		t.Fatalf("parse capability.go: %v", err)
	}
	found := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "Task" {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					continue
				}
				found[name.Name] = strings.Trim(lit.Value, `"`)
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("no Task constants found; the AST walk is broken, not the registry")
	}
	return found
}

func TestEveryDeclaredTaskIsRegistered(t *testing.T) {
	t.Parallel()
	for name, taskID := range declaredTasks(t) {
		if _, ok := Registry[Task(taskID)]; !ok {
			t.Errorf("Task %s (%q) is declared but missing from Registry", name, taskID)
		}
	}
}

func TestEveryRegisteredTaskIsDeclared(t *testing.T) {
	t.Parallel()
	declared := declaredTasks(t)
	declaredSet := map[string]bool{}
	for _, taskID := range declared {
		declaredSet[taskID] = true
	}
	for task := range Registry {
		if !declaredSet[string(task)] {
			t.Errorf("Task %q is in Registry but has no constant declared", task)
		}
	}
}

func TestRegistrySpecsAreValid(t *testing.T) {
	t.Parallel()
	for task, spec := range Registry {
		if spec.Category == "" {
			t.Errorf("Task %q has empty Category", task)
		}
		if spec.DeliveryContract == "" {
			t.Errorf("Task %q has empty DeliveryContract", task)
		}
		// OutputModalities may be empty for some tasks (e.g. observations)
		// but at least one modality dimension should be present
		if len(spec.InputModalities) == 0 && len(spec.OutputModalities) == 0 {
			t.Errorf("Task %q has both InputModalities and OutputModalities empty", task)
		}
	}
}

func TestIsValid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		task Task
		want bool
	}{
		{ImageClassification, true},
		{TextToImage, true},
		{AutomaticSpeechRecognition, true},
		{Task("nonexistent-task"), false},
		{Task(""), false},
	}
	for _, tc := range cases {
		got := IsValid(tc.task)
		if got != tc.want {
			t.Errorf("IsValid(%q) = %v, want %v", tc.task, got, tc.want)
		}
	}
}

func TestTasksReturnsAll(t *testing.T) {
	t.Parallel()
	tasks := Tasks()
	if len(tasks) != len(Registry) {
		t.Errorf("Tasks() returned %d tasks, Registry has %d", len(tasks), len(Registry))
	}
	// Verify no duplicates
	seen := map[Task]bool{}
	for _, task := range tasks {
		if seen[task] {
			t.Errorf("Tasks() returned duplicate: %q", task)
		}
		seen[task] = true
	}
}

func TestCategoriesReturnsExpected(t *testing.T) {
	t.Parallel()
	cats := Categories()
	expected := []Category{
		CategoryMultimodal,
		CategoryComputerVision,
		CategoryNLP,
		CategoryAudio,
		CategoryTabular,
		CategoryRL,
	}
	if len(cats) != len(expected) {
		t.Errorf("Categories() returned %d, expected %d", len(cats), len(expected))
	}
	// Verify all expected are present
	found := map[Category]bool{}
	for _, c := range cats {
		found[c] = true
	}
	for _, exp := range expected {
		if !found[exp] {
			t.Errorf("Categories() missing %q", exp)
		}
	}
}

func TestContractConstants(t *testing.T) {
	t.Parallel()
	// Lock the contract identifiers so they cannot drift silently
	if ContractChange != "change" {
		t.Errorf("ContractChange = %q, want %q", ContractChange, "change")
	}
	if ContractArtifact != "artifact" {
		t.Errorf("ContractArtifact = %q, want %q", ContractArtifact, "artifact")
	}
	if ContractObservation != "observation" {
		t.Errorf("ContractObservation = %q, want %q", ContractObservation, "observation")
	}
}

func TestModalityConstants(t *testing.T) {
	t.Parallel()
	// Lock the modality identifiers for alignment with Draft 41
	if ModalityText != "text" {
		t.Errorf("ModalityText = %q, want %q", ModalityText, "text")
	}
	if ModalityImage != "image" {
		t.Errorf("ModalityImage = %q, want %q", ModalityImage, "image")
	}
	if ModalityVideo != "video" {
		t.Errorf("ModalityVideo = %q, want %q", ModalityVideo, "video")
	}
	if ModalityAudio != "audio" {
		t.Errorf("ModalityAudio = %q, want %q", ModalityAudio, "audio")
	}
}
