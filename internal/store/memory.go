package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/core"
)

// interface conformance
var _ Store = (*MemoryStore)(nil)

type MemoryStore struct {
	mu   sync.RWMutex
	jobs map[string]*core.Job
}

func NewMemoryStore() Store {
	return &MemoryStore{
		jobs: make(map[string]*core.Job),
	}
}

func (s *MemoryStore) SaveJob(job *core.Job) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[job.ID] = job
}

func (s *MemoryStore) GetJob(id string) (*core.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, exists := s.jobs[id]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}
	return job, nil
}

// updateTarget applies fn to the target for a job and arch and bumps UpdatedAt
func (s *MemoryStore) UpdateTarget(jobID, arch string, fn func(j *core.Job, t *core.JobTarget)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]

	if !ok {
		return
	}

	for _, t := range job.Targets {
		if t.Arch == arch {
			fn(job, t)
			job.UpdatedAt = time.Now()
			return
		}
	}
}

// RecalculateJobStatus recomputes the overall job.Status from the target statuses
func (s *MemoryStore) RecalculateJobStatus(jobID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[jobID]
	if !ok {
		return fmt.Errorf("job not found: %s", jobID)
	}

	job.RecalculateJobStatus()
	s.SaveJob(job)

	return nil
}

// ListJobs returns all jobs in the store
func (s *MemoryStore) ListJobs() ([]*core.Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	jobs := make([]*core.Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job)
	}
	return jobs, nil
}
