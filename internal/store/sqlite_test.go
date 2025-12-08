package store

import (
	"testing"
	"time"

	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/core"
	"github.com/stretchr/testify/assert"
)

// Add to internal/store/sqlite_test.go
func TestSQLiteStore(t *testing.T) {
	s := NewSQLiteStore("test.db")
	job := &core.Job{ID: "test"}
	s.SaveJob(job)
	got, _ := s.GetJob("test")
	assert.Equal(t, "test", got.ID)
}
func TestSQLiteStoreGetJob(t *testing.T) {
	s := NewSQLiteStore("test.db")

	// Test missing job - EXPECT "no rows in result set"
	_, err := s.GetJob("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no rows in result set")

	// Test save + get
	job := &core.Job{
		ID:   "test-job",
		Repo: "test/repo",
	}
	s.SaveJob(job)

	got, err := s.GetJob("test-job")
	assert.NoError(t, err)
	assert.Equal(t, "test-job", got.ID)
}
func TestSQLiteStoreSaveAndGetJob(t *testing.T) {
	s := NewSQLiteStore("file::memory:?cache=shared") // Memory DB for test

	job := &core.Job{
		ID:            "test-job",
		Repo:          "github.com/test/repo",
		Commit:        "abc123",
		TestCommand:   "go test ./...",
		Architectures: []string{"amd64"},
		Status:        core.JobStatusPending,
		CreatedAt:     time.Now().UTC(),
	}

	saved, err := s.SaveJob(job)
	if err != nil {
		t.Fatalf("SaveJob error: %v", err)
	}
	if saved.ID != job.ID {
		t.Errorf("Saved job ID mismatch: got %v, want %v", saved.ID, job.ID)
	}

	fetched, err := s.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob error: %v", err)
	}
	if fetched.ID != job.ID {
		t.Errorf("GetJob ID mismatch: got %v, want %v", fetched.ID, job.ID)
	}
}
func TestSQLiteStoreListJobs(t *testing.T) {
	s := NewSQLiteStore("file::memory:?cache=shared")

	job := &core.Job{ID: "job1", Repo: "repo"}
	target := &core.JobTarget{Arch: "amd64", Status: core.TargetStatusPassed}
	job.Targets = append(job.Targets, target)
	s.SaveJob(job)

	jobs, err := s.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs error: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("expected at least one job")
	}
	found := false
	for _, j := range jobs {
		if j.ID == job.ID && len(j.Targets) == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("Saved job with targets not found")
	}
}
func TestSQLiteStoreRecalculateJobStatus(t *testing.T) {
	s := NewSQLiteStore("file::memory:?cache=shared")

	job := &core.Job{ID: "job2", Status: core.JobStatusPending}
	s.SaveJob(job)

	err := s.RecalculateJobStatus(job.ID)
	if err != nil {
		t.Fatalf("RecalculateJobStatus error: %v", err)
	}

	got, _ := s.GetJob(job.ID)
	if got.Status != core.JobStatusPending {
		t.Errorf("Unexpected job status: got %v, want %v", got.Status, core.JobStatusPending)
	}
}
func TestSQLiteStoreJobNotFound(t *testing.T) {
	s := NewSQLiteStore("file::memory:?cache=shared")

	_, err := s.GetJob("missing")
	if err == nil {
		t.Fatal("Expected error for missing job, got nil")
	}
	if err.Error() != "sql: no rows in result set" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestSQLiteStoreUpdateTarget(t *testing.T) {
	s := NewSQLiteStore("file::memory:?cache=shared")

	// Create job with target
	job := &core.Job{
		ID:      "update-test",
		Repo:    "test/repo",
		Targets: []*core.JobTarget{{Arch: "amd64", Status: core.TargetStatusPending}},
	}
	s.SaveJob(job)

	// Update target status
	err := s.UpdateTarget(job.ID, "amd64", func(j *core.Job, t *core.JobTarget) {
		t.Status = core.TargetStatusPassed
	})
	assert.NoError(t, err)

	// Verify update persisted
	updated, err := s.GetJob(job.ID)
	assert.NoError(t, err)
	assert.Equal(t, core.TargetStatusPassed, updated.Targets[0].Status)
}
