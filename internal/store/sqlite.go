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
	var startedAt, endedAt sql.NullString
	var architecturesStr string

	err := s.db.QueryRow(`
		SELECT id, repo, commit_hash, test_command, architectures, status, 
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
	if startedAt.Valid && startedAt.String != "" {
		ts, _ := time.Parse(time.RFC3339, startedAt.String)
		job.StartedAt = &ts
	}
	if endedAt.Valid && endedAt.String != "" {
		ts, _ := time.Parse(time.RFC3339, endedAt.String)
		job.EndedAt = &ts
	}

	rows, _ := s.db.Query(`
		SELECT arch, status, reason, log, exit_code, started_at, ended_at
		FROM job_targets WHERE job_id = ?`, id)
	defer rows.Close()

	for rows.Next() {
		t := &core.JobTarget{}
		var startedAtStr, endedAtStr sql.NullString
		rows.Scan(&t.Arch, &t.Status, &t.Reason, &t.Log, &t.ExitCode, &startedAt, &endedAt)
		if startedAtStr.Valid && startedAtStr.String != "" {
			ts, _ := time.Parse(time.RFC3339, startedAtStr.String)
			t.StartedAt = &ts
		}
		if endedAtStr.Valid && endedAtStr.String != "" {
			ts, _ := time.Parse(time.RFC3339, endedAtStr.String)
			t.EndedAt = &ts
		}
		job.Targets = append(job.Targets, t)
	}

	return job, nil
}

// ListJobs implements Store.
func (s *SQLiteStore) ListJobs() ([]*core.Job, error) {
	rows, err := s.db.Query(`
        SELECT id, repo, commit_hash, test_command, architectures, status,
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
	job := &core.Job{}
	var createdAt, updatedAt string
	var startedAt, endedAt sql.NullString
	var architecturesStr string

	err := s.db.QueryRow(`
		SELECT id, repo, commit_hash, test_command, architectures, status, 
		       created_at, updated_at, started_at, ended_at, timeout
		FROM jobs WHERE id = ?`, jobID).Scan(
		&job.ID, &job.Repo, &job.Commit, &job.TestCommand,
		&architecturesStr, &job.Status, &createdAt, &updatedAt,
		&startedAt, &endedAt, &job.Timeout)

	if err != nil {
		return err
	}

	if startedAt.Valid && startedAt.String != "" {
		ts, err := time.Parse(time.RFC3339, startedAt.String)
		if err == nil {
			job.StartedAt = &ts
		}
	}

	if endedAt.Valid && endedAt.String != "" {
		ts, err := time.Parse(time.RFC3339, endedAt.String)
		if err == nil {
			job.EndedAt = &ts
		}
	}
	job.RecalculateJobStatus()
	s.SaveJob(job)

	return nil
}

// SaveJob implements Store.
func (s *SQLiteStore) SaveJob(job *core.Job) (*core.Job, error) {
	tx, _ := s.db.Begin()
	defer tx.Rollback()

	// upsert job
	if _, err := tx.Exec(`
		INSERT OR REPLACE INTO jobs 
		(id, repo, commit_hash, test_command, architectures, status, created_at, updated_at, 
		 started_at, ended_at, timeout, env)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.Repo, job.Commit, job.TestCommand,
		strings.Join(job.Architectures, ","), job.Status,
		formatTimePtr(&job.CreatedAt), formatTimePtr(&job.UpdatedAt),
		formatTimePtr(job.StartedAt), formatTimePtr(job.EndedAt),
		job.Timeout, ""); err != nil {
		return nil, fmt.Errorf("upsert job: %w", err)
	}

	// Delete old targets
	_, _ = tx.Exec(`DELETE FROM job_targets WHERE job_id = ?`, job.ID)

	// Insert targets
	for _, t := range job.Targets {
		if _, err := tx.Exec(`
			INSERT INTO job_targets (job_id, arch, status, reason, log, exit_code, started_at, ended_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			job.ID, t.Arch, t.Status, t.Reason, t.Log, t.ExitCode,
			formatTimePtr(t.StartedAt), formatTimePtr(t.EndedAt)); err != nil {
			return nil, fmt.Errorf("insert target: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return job, nil
}

// UpdateTarget implements Store.
// UpdateTarget implements Store.
func (s *SQLiteStore) UpdateTarget(jobID string, arch string, fn func(j *core.Job, t *core.JobTarget)) error {
	// Step 1: Fetch the job
	job, err := s.GetJob(jobID)
	if err != nil {
		return fmt.Errorf("get job %s: %w", jobID, err)
	}

	// Step 2: Find the target and apply the update function
	targetFound := false
	for i := range job.Targets {
		if job.Targets[i].Arch == arch {
			fn(job, job.Targets[i]) // Apply the update callback
			targetFound = true
			break
		}
	}

	if !targetFound {
		return fmt.Errorf("target %s not found for job %s", arch, jobID)
	}

	// Step 3: Save the updated job back to DB
	_, err = s.SaveJob(job)
	if err != nil {
		return fmt.Errorf("save updated job %s: %w", jobID, err)
	}

	return nil
}

func NewSQLiteStore(path string) Store {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		panic(fmt.Errorf("open db: %w", err))
	}

	// Auto-migrate WITH error checking
	if _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS jobs (
            id TEXT PRIMARY KEY,
            repo TEXT NOT NULL,
            commit_hash TEXT NOT NULL,  -- Fixed column name
            test_command TEXT NOT NULL,
            architectures TEXT NOT NULL,
            status TEXT NOT NULL,
            created_at TEXT NOT NULL DEFAULT current_timestamp,
            updated_at TEXT NOT NULL DEFAULT current_timestamp,
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
    `); err != nil {
		panic(fmt.Errorf("migration failed: %w", err))
	}

	return &SQLiteStore{db: db}
}

// formatTimePtr formats a time pointer to RFC3339 or returns nil if the pointer is nil
func formatTimePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339)
}
