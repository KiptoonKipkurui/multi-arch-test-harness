package store

import (
	"testing"

	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestMemoryStoreGetJob(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*MemoryStore)
		id      string
		want    *core.Job
		wantErr bool
	}{
		{
			name: "job exists",
			setup: func(s *MemoryStore) {
				s.SaveJob(&core.Job{
					ID:     "test-job-1",
					Repo:   "test/repo",
					Status: core.JobStatusPassed,
				})
			},
			id: "test-job-1",
			want: &core.Job{
				ID:     "test-job-1",
				Repo:   "test/repo",
				Status: core.JobStatusPassed,
			},
			wantErr: false,
		},
		{
			name:    "job not found",
			id:      "missing",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewMemoryStore().(*MemoryStore)
			if tt.setup != nil {
				tt.setup(s)
			}

			got, err := Store(s).GetJob(tt.id)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return // ← FIX: Exit immediately
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want.ID, got.ID)
		})
	}
}

func TestMemoryStoreGetJobConcurrent(t *testing.T) {
	s := NewMemoryStore()

	// Add job
	job := &core.Job{ID: "concurrent-job", Status: core.JobStatusPending}
	s.SaveJob(job)

	// Concurrent reads
	const numGoroutines = 10
	const numReads = 100

	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			for j := 0; j < numReads; j++ {
				_, err := s.GetJob("concurrent-job")
				if err != nil {
					errCh <- err
					return
				}
			}
			errCh <- nil
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		err := <-errCh
		assert.NoError(t, err)
	}
}
