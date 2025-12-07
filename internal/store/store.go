package store

import "github.com/kiptoonkipkurui/multi-arch-test-harness/internal/core"

type Store interface {
	SaveJob(job *core.Job)
	GetJob(id string) (*core.Job, error)
	UpdateTarget(jobID, arch string, fn func(j *core.Job, t *core.JobTarget))
	RecalculateJobStatus(jobID string) error
	ListJobs() ([]*core.Job, error)
}

type StoreBuilder struct {
	store Store
}

func NewStoreBuilder() *StoreBuilder {
	return &StoreBuilder{}
}
func (b *StoreBuilder) WithMemoryStore() *StoreBuilder {
	b.store = NewMemoryStore()
	return b
}
func (b *StoreBuilder) WithSQLite(path string) *StoreBuilder {
	b.store = NewSQLiteStore(path)
	return b
}

func (b *StoreBuilder) Build() Store {
	return b.store
}
