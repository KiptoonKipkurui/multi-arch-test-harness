package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/core"
)

// interface conformance
var _ Store = (*SQLiteStore)(nil)

type SQLiteStore struct {
	db *sql.DB
}

// GetJob implements Store.
func (s *SQLiteStore) GetJob(id string) (*core.Job, error) {
	job := &core.Job{}
	var createdAt, updatedAt string
	var startedAt, endedAt, architecturesStr string

	err := s.db.QueryRow(`
		SELECT id, repo, commit, test_command, architectures, status, 
		       created_at, updated_at, started_at, ended_at, timeout
		FROM jobs WHERE id = ?`, id).Scan(
		&job.ID, &job.Repo, &job.Commit, &job.TestCommand,
		&architecturesStr, &job.Status, &createdAt, &updatedAt,
		&startedAt, &endedAt, &job.Timeout)

	if err != nil {
		return nil, err
	}

	job.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	job.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	job.Architectures = strings.Split(architecturesStr, ",")
	if startedAt != "" {
		ts, _ := time.Parse(time.RFC3339, startedAt)
		job.StartedAt = &ts
	}
	if endedAt != "" {
		ts, _ := time.Parse(time.RFC3339, endedAt)
		job.EndedAt = &ts
	}

	rows, _ := s.db.Query(`
		SELECT arch, status, reason, log, exit_code, started_at, ended_at
		FROM job_targets WHERE job_id = ?`, id)
	defer rows.Close()

	for rows.Next() {
		t := &core.JobTarget{}
		var startedAtStr, endedAtStr string
		rows.Scan(&t.Arch, &t.Status, &t.Reason, &t.Log, &t.ExitCode, &startedAt, &endedAt)
		if startedAtStr != "" {
			ts, _ := time.Parse(time.RFC3339, startedAtStr)
			t.StartedAt = &ts
		}
		if endedAtStr != "" {
			ts, _ := time.Parse(time.RFC3339, endedAtStr)
			t.EndedAt = &ts
		}
		job.Targets = append(job.Targets, t)
	}

	return job, nil
}

// ListJobs implements Store.
func (s *SQLiteStore) ListJobs() ([]*core.Job, error) {
	rows, err := s.db.Query(`
        SELECT id, repo, commit, test_command, architectures, status,
               created_at, updated_at, started_at, ended_at, timeout
        FROM jobs
        ORDER BY created_at DESC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]*core.Job, 0)
	jobByID := make(map[string]*core.Job)

	for rows.Next() {
		var (
			id            string
			repo          string
			commit        string
			testCommand   string
			architectures string
			status        string
			createdAtStr  string
			updatedAtStr  string
			startedAtStr  sql.NullString
			endedAtStr    sql.NullString
			timeout       sql.NullString
		)

		if err := rows.Scan(
			&id, &repo, &commit, &testCommand,
			&architectures, &status,
			&createdAtStr, &updatedAtStr,
			&startedAtStr, &endedAtStr,
			&timeout,
		); err != nil {
			return nil, err
		}

		createdAt, _ := time.Parse(time.RFC3339, createdAtStr)
		updatedAt, _ := time.Parse(time.RFC3339, updatedAtStr)

		var startedAtPtr *time.Time
		if startedAtStr.Valid && startedAtStr.String != "" {
			ts, err := time.Parse(time.RFC3339, startedAtStr.String)
			if err == nil {
				startedAtPtr = &ts
			}
		}

		var endedAtPtr *time.Time
		if endedAtStr.Valid && endedAtStr.String != "" {
			ts, err := time.Parse(time.RFC3339, endedAtStr.String)
			if err == nil {
				endedAtPtr = &ts
			}
		}

		job := &core.Job{
			ID:            id,
			Repo:          repo,
			Commit:        commit,
			TestCommand:   testCommand,
			Architectures: strings.Split(architectures, ","),
			Status:        core.JobStatus(status),
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
			StartedAt:     startedAtPtr,
			EndedAt:       endedAtPtr,
		}
		if timeout.Valid {
			job.Timeout = timeout.String
		}

		jobs = append(jobs, job)
		jobByID[id] = job
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Fetch targets for all jobs in one query
	if len(jobs) == 0 {
		return jobs, nil
	}

	ids := make([]interface{}, 0, len(jobs))
	placeholders := make([]string, 0, len(jobs))
	for _, j := range jobs {
		ids = append(ids, j.ID)
		placeholders = append(placeholders, "?")
	}

	targetRows, err := s.db.Query(
		fmt.Sprintf(`
            SELECT job_id, arch, status, reason, log, exit_code, started_at, ended_at
            FROM job_targets
            WHERE job_id IN (%s)
        `, strings.Join(placeholders, ",")),
		ids...,
	)
	if err != nil {
		return nil, err
	}
	defer targetRows.Close()

	for targetRows.Next() {
		var (
			jobID        string
			arch         string
			status       string
			reason       sql.NullString
			logText      sql.NullString
			exitCode     sql.NullInt64
			startedAtStr sql.NullString
			endedAtStr   sql.NullString
		)

		if err := targetRows.Scan(
			&jobID, &arch, &status, &reason, &logText, &exitCode,
			&startedAtStr, &endedAtStr,
		); err != nil {
			return nil, err
		}

		job := jobByID[jobID]
		if job == nil {
			continue
		}

		t := &core.JobTarget{
			Arch:   arch,
			Status: core.TargetStatus(status),
		}
		if reason.Valid {
			t.Reason = reason.String
		}
		if logText.Valid {
			t.Log = logText.String
		}
		if exitCode.Valid {
			t.ExitCode = int(exitCode.Int64)
		}
		if startedAtStr.Valid && startedAtStr.String != "" {
			ts, err := time.Parse(time.RFC3339, startedAtStr.String)
			if err == nil {
				t.StartedAt = &ts
			}
		}
		if endedAtStr.Valid && endedAtStr.String != "" {
			ts, err := time.Parse(time.RFC3339, endedAtStr.String)
			if err == nil {
				t.EndedAt = &ts
			}
		}

		job.Targets = append(job.Targets, t)
	}

	if err := targetRows.Err(); err != nil {
		return nil, err
	}

	return jobs, nil
}

// RecalculateJobStatus implements Store.
func (s *SQLiteStore) RecalculateJobStatus(jobID string) error {
	panic("unimplemented")
}

// SaveJob implements Store.
func (s *SQLiteStore) SaveJob(job *core.Job) {
	tx, _ := s.db.Begin()
	defer tx.Rollback()

	// upsert job
	_, _ = tx.Exec(`
		INSERT OR REPLACE INTO jobs 
		(id, repo, commit, test_command, architectures, status, created_at, updated_at, 
		 started_at, ended_at, timeout, env)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Repo, job.Commit, job.TestCommand,
		strings.Join(job.Architectures, ","), job.Status,
		job.CreatedAt.Format(time.RFC3339), job.UpdatedAt.Format(time.RFC3339),
		job.StartedAt.Format(time.RFC3339), job.EndedAt.Format(time.RFC3339),
		job.Timeout, "")

	// Delete old targets
	_, _ = tx.Exec(`DELETE FROM job_targets WHERE job_id = ?`, job.ID)

	// Insert targets
	for _, t := range job.Targets {
		_, _ = tx.Exec(`
			INSERT INTO job_targets (job_id, arch, status, reason, log, exit_code, started_at, ended_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			job.ID, t.Arch, t.Status, t.Reason, t.Log, t.ExitCode,
			t.StartedAt.Format(time.RFC3339), t.EndedAt.Format(time.RFC3339))
	}

	tx.Commit()
}

// UpdateTarget implements Store.
func (s *SQLiteStore) UpdateTarget(jobID string, arch string, fn func(j *core.Job, t *core.JobTarget)) {
	panic("unimplemented")
}

func NewSQLiteStore(path string) Store {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		panic(err)
	}
	// Auto-migrate
	_, _ = db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			repo TEXT NOT NULL,
			commit TEXT NOT NULL,
			test_command TEXT NOT NULL,
			architectures TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			started_at TEXT,
			ended_at TEXT,
			timeout TEXT,
			env TEXT
		);
		
		CREATE TABLE IF NOT EXISTS job_targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id TEXT NOT NULL,
			arch TEXT NOT NULL,
			status TEXT NOT NULL,
			reason TEXT,
			log TEXT,
			exit_code INTEGER,
			started_at TEXT,
			ended_at TEXT,
			UNIQUE(job_id, arch),
			FOREIGN KEY(job_id) REFERENCES jobs(id)
		);
	`)

	return &SQLiteStore{db: db}
}
