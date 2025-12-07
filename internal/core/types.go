package core

import (
	"time"
)

type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusPassed  JobStatus = "passed"
	JobStatusFailed  JobStatus = "failed"
	JobStatusError   JobStatus = "error"
)

type TargetStatus string

const (
	TargetStatusPending TargetStatus = "pending"
	TargetStatusRunning TargetStatus = "running"
	TargetStatusPassed  TargetStatus = "passed"
	TargetStatusFailed  TargetStatus = "failed"
	TargetStatusError   TargetStatus = "error"
)

type Job struct {
	ID            string            `json:"id"`
	Repo          string            `json:"repo"`
	Commit        string            `json:"commit"`
	TestCommand   string            `json:"test_command"`
	Architectures []string          `json:"architectures"`
	Status        JobStatus         `json:"status"`
	Targets       []*JobTarget      `json:"targets"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
	StartedAt     *time.Time        `json:"started_at,omitempty"`
	EndedAt       *time.Time        `json:"ended_at,omitempty"`
	Timeout       string            `json:"timeout,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
}

type JobTarget struct {
	Arch      string            `json:"arch"`
	Status    TargetStatus      `json:"status"`
	Reason    string            `json:"reason,omitempty"` //"docker_error", "git_error", "tests_failed"
	Log       string            `json:"log,omitempty"`
	ExitCode  int               `json:"exit_code"`
	StartedAt *time.Time        `json:"started_at,omitempty"`
	EndedAt   *time.Time        `json:"ended_at,omitempty"`
	Timeout   string            `json:"timeout,omitempty"` // "5m", "30s"
	Env       map[string]string `json:"env,omitempty"`     // pass to docker -e
}

// RecalculateJobStatus recomputes the overall job.Status from the target statuses
func (job *Job) RecalculateJobStatus() {

	allPassed := true
	anyRunning := false
	anyFailedOrError := false

	for _, t := range job.Targets {
		switch t.Status {
		case TargetStatusRunning, TargetStatusPending:
			anyRunning = true
			allPassed = false
		case TargetStatusFailed, TargetStatusError:
			anyFailedOrError = true
			allPassed = false
		case TargetStatusPassed:
			// do nothing
		}
	}

	switch {
	case anyRunning:
		job.Status = JobStatusRunning
	case anyFailedOrError:
		job.Status = JobStatusFailed
	case allPassed:
		job.Status = JobStatusPassed
	default:
		job.Status = JobStatusPending
	}
	deriveJobTimes(job)
	job.UpdatedAt = time.Now()
}
func deriveJobTimes(job *Job) {
	var (
		start *time.Time
		end   *time.Time
	)

	for _, t := range job.Targets {
		if t.StartedAt != nil {
			if start == nil || t.StartedAt.Before(*start) {
				start = t.StartedAt
			}
		}

		if t.EndedAt != nil {
			if end == nil || t.EndedAt.After(*end) {
				end = t.EndedAt
			}
		}
	}
	job.StartedAt = start
	job.EndedAt = end
}
