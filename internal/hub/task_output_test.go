package hub

import (
	"testing"
	"time"

	"github.com/pleware/initagent/internal/offering"
	"github.com/pleware/initagent/internal/orgplan"
)

func TestOpenStoreCreatesTaskOutputs(t *testing.T) {
	s := testStore(t)
	ok, err := s.hasTable("task_outputs")
	if err != nil || !ok {
		t.Fatalf("task_outputs missing: ok=%v err=%v", ok, err)
	}
}

func TestSaveAndGetTaskOutput(t *testing.T) {
	s := testStore(t)
	org, err := s.CreateOrg("Logs")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if err := s.SaveTaskOutput(TaskOutput{
		TaskID: "task-1", OrgID: org.Id, ProjectID: "project-1",
		Stdout: "hi\n", Stderr: "warn\n", CreatedAt: now.Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.TaskOutputByID("task-1")
	if err != nil || got == nil {
		t.Fatalf("get: %v %+v", err, got)
	}
	if got.Stdout != "hi\n" || got.Stderr != "warn\n" || got.OrgID != org.Id {
		t.Fatalf("got = %+v", got)
	}
	if err := s.SaveTaskOutput(TaskOutput{
		TaskID: "task-1", OrgID: org.Id, ProjectID: "project-1",
		Stdout: "again\n", CreatedAt: now.Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	again, err := s.TaskOutputByID("task-1")
	if err != nil || again.Stdout != "again\n" {
		t.Fatalf("replace: %v %+v", err, again)
	}
}

func TestPurgeTaskOutputsRespectsPlan(t *testing.T) {
	s := testStore(t)
	s.setOffering(offering.Hosted)
	org, err := s.CreateOrg("Free logs")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	old := now.Add(-8 * 24 * time.Hour).Unix()
	fresh := now.Add(-2 * 24 * time.Hour).Unix()
	if err := s.SaveTaskOutput(TaskOutput{TaskID: "task-old", OrgID: org.Id, ProjectID: "project-1", Stdout: "old", CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTaskOutput(TaskOutput{TaskID: "task-new", OrgID: org.Id, ProjectID: "project-1", Stdout: "new", CreatedAt: fresh}); err != nil {
		t.Fatal(err)
	}
	n, err := s.PurgeTaskOutputs(now)
	if err != nil || n != 1 {
		t.Fatalf("purge hosted free = %d, %v", n, err)
	}
	if got, _ := s.TaskOutputByID("task-old"); got != nil {
		t.Fatal("old row should be gone")
	}
	if got, _ := s.TaskOutputByID("task-new"); got == nil || got.Stdout != "new" {
		t.Fatal("fresh row should stay")
	}

	if err := s.SetOrgPlan(org.Id, orgplan.Enterprise); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveTaskOutput(TaskOutput{TaskID: "task-mid", OrgID: org.Id, ProjectID: "project-1", Stdout: "mid", CreatedAt: now.Add(-30 * 24 * time.Hour).Unix()}); err != nil {
		t.Fatal(err)
	}
	n, err = s.PurgeTaskOutputs(now)
	if err != nil || n != 0 {
		t.Fatalf("enterprise 30-day row must stay: %d %v", n, err)
	}
}

func TestPurgeTaskOutputsSelfHostKeeps(t *testing.T) {
	s := testStore(t)
	s.setOffering(offering.Selfhost)
	org, err := s.CreateOrg("OSS")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	if err := s.SaveTaskOutput(TaskOutput{
		TaskID: "task-old", OrgID: org.Id, ProjectID: "project-1",
		Stdout: "old", CreatedAt: now.Add(-400 * 24 * time.Hour).Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	n, err := s.PurgeTaskOutputs(now)
	if err != nil || n != 0 {
		t.Fatalf("self-host purge = %d, %v", n, err)
	}
}
