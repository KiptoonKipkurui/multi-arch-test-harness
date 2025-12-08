// API: Create new test job
// @title Multi-Arch Test Harness API
// @version 1.0
// @description Distributed multi-arch test runner for CI/CD
// @host localhost:8080
// @BasePath /

package api

import (
	"encoding/json"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/core"
	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/logging"
	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/runner"
	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/store"
)

type Server struct {
	store      store.Store
	runner     *runner.Runner
	httpServer *http.Server
	jobCounter uint64
}
type jobTargetView struct {
	Arch      string            `json:"arch"`
	Status    core.TargetStatus `json:"status"`
	ExitCode  int               `json:"exit_code"`
	StartedAt *time.Time        `json:"started_at,omitempty"`
	EndedAt   *time.Time        `json:"ended_at,omitempty"`
	Log       string            `json:"log,omitempty"`
}

type jobView struct {
	ID            string          `json:"id"`
	Repo          string          `json:"repo"`
	Commit        string          `json:"commit"`
	TestCommand   string          `json:"test_command"`
	Architectures []string        `json:"architectures"`
	Status        core.JobStatus  `json:"status"`
	Targets       []jobTargetView `json:"targets"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
	StartedAt     *time.Time      `json:"started_at,omitempty"`
	EndedAt       *time.Time      `json:"ended_at,omitempty"`
}

func NewServer(addr string, st store.Store) *Server {
	s := &Server{
		store:  st,
		runner: runner.NewRunner(st)}
	mux := http.NewServeMux()

	mux.HandleFunc("/jobs", s.handleJobs)
	mux.HandleFunc("/jobs/", s.routeJob)

	// Health + metrics
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/metrics", s.handleMetrics)

	// API docs
	// API docs - use Handle(), NOT HandleFunc()
	mux.Handle("/docs/", http.StripPrefix("/docs/", http.FileServer(http.Dir("./docs"))))
	mux.Handle("/openapi.yaml", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./docs/swagger.yaml") // or ./docs/api/openapi.yaml
	}))
	mux.Handle("/", recoveryMiddleware(mux))

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	logging.Logger.Info("server_starting", "addr", addr)
	return s
}
func (s *Server) Start() error {
	log.Printf("listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

type createJobRequest struct {
	Repo          string            `json:"repo"`
	Commit        string            `json:"commit"`
	TestCommand   string            `json:"test_command"`
	Architectures []string          `json:"architectures"`
	Timeout       string            `json:"timeout,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
}

type createJobResponse struct {
	ID string `json:"id"`
}

// @Summary Create new test job
// @Description Triggers multi-arch test execution across architectures
// @Tags jobs
// @Accept json
// @Produce json
// @Param body body createJobRequest true "Test job specification"
// @Success 200 {object} createJobResponse
// @Router /jobs [post]
func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createJob(w, r)
	case http.MethodGet:
		s.listJobs(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// API: Create new test job
// @summary Create a new multi-arch test job
// @description Triggers test execution across specified architectures
// @tags jobs
// @accept json
// @produce json
// @param job body createJobRequest true "Job specification"
// @success 200 {object} createJobResponse
// @router /jobs [post]
func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if len(req.Architectures) == 0 || req.Repo == "" || req.TestCommand == "" {
		http.Error(w, "missing required fields", http.StatusBadRequest)
		return
	}

	now := time.Now()
	jobID := s.newJobID()
	targets := make([]*core.JobTarget, 0, len(req.Architectures))
	for _, arch := range req.Architectures {
		targets = append(targets, &core.JobTarget{
			Arch:   arch,
			Status: core.TargetStatusPending,
		})
	}
	job := &core.Job{
		ID:            jobID,
		Repo:          req.Repo,
		Commit:        req.Commit,
		TestCommand:   req.TestCommand,
		Architectures: req.Architectures,
		Status:        core.JobStatusPending,
		Targets:       targets,
		CreatedAt:     now,
		UpdatedAt:     now,
		Timeout:       req.Timeout,
		Env:           req.Env,
	}
	s.store.SaveJob(job)
	logging.Logger.Info("job_created",
		"job_id", jobID,
		"repo", req.Repo,
		"commit", req.Commit,
		"architectures", req.Architectures,
	)

	// Kick off async execution
	s.runner.RunJobAsync(job)

	// For now, we are not running the job yet
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(createJobResponse{ID: jobID})
}

func (s *Server) newJobID() string {
	n := atomic.AddUint64(&s.jobCounter, 1)
	return time.Now().Format("20060102T150405") + "-" + randomString(4) + "-" + string(rune(n%10+'0'))
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}

	return string(b)
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// URL format is /jobs/{id}
	id := r.URL.Path[len("/jobs/"):]
	job, err := s.store.GetJob(id)

	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	logging.Logger.Info("job_fetched", "job_id", id, "status", job.Status)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toJobView(job))
}

func (s *Server) routeJob(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path[len("/jobs/"):] // everything after /jobs/

	// log endpoint pattern: {id}/targets/{arch}/log
	if strings.Contains(path, "/targets/") && strings.HasSuffix(path, "/log") {
		s.handleTargetLog(w, r, path)
		return
	}

	// default: /jobs/{id}
	s.handleJobByIDPath(w, r, path)
}

// @Summary Get job status
// @Description Fetches job details with per-target results (truncated logs)
// @Tags jobs
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} jobView
// @Failure 404 {string} string "Job not found"
// @Router /jobs/{id} [get]
func (s *Server) handleJobByIDPath(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	job, err := s.store.GetJob(id)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	logging.Logger.Info("job_fetched", "job_id", id, "status", job.Status)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toJobView(job))
}

func (s *Server) handleTargetLog(w http.ResponseWriter, r *http.Request, path string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// path format: {id}/targets/{arch}/log
	parts := strings.Split(path, "/")
	if len(parts) != 4 || parts[1] != "targets" || parts[3] != "log" {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	jobID := parts[0]
	arch := parts[2]

	job, err := s.store.GetJob(jobID)
	if err != nil {
		http.Error(w, "job not found", http.StatusNotFound)
		return
	}
	var target *core.JobTarget
	for _, t := range job.Targets {
		if t.Arch == arch {
			target = t
			break
		}
	}
	if target == nil {
		http.Error(w, "target not found", http.StatusNotFound)
		return
	}

	logging.Logger.Info("target_log_fetched", "job_id", jobID, "arch", arch)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if target.Log == "" {
		_, _ = w.Write([]byte("(no log)\n"))
		return
	}
	_, _ = w.Write([]byte(target.Log))
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	jobs, _ := s.store.ListJobs()

	views := make([]jobView, 0, len(jobs))
	for _, j := range jobs {
		if statusFilter != "" && string(j.Status) != statusFilter {
			continue
		}
		views = append(views, toJobView(j))
	}

	logging.Logger.Info("jobs_listed", "count", len(views), "status_filter", statusFilter)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(views)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Stub for now
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte("# TODO: metrics\n"))
}

const maxTargetLogPreview = 512

func toJobView(job *core.Job) jobView {
	targets := make([]jobTargetView, 0, len(job.Targets))
	for _, t := range job.Targets {
		view := jobTargetView{
			Arch:      t.Arch,
			Status:    t.Status,
			ExitCode:  t.ExitCode,
			StartedAt: t.StartedAt,
			EndedAt:   t.EndedAt,
		}
		// Optional: include a preview of logs, truncated
		if t.Log != "" {
			logStr := t.Log
			if len(logStr) > maxTargetLogPreview {
				logStr = logStr[:maxTargetLogPreview] + "...(truncated)"
			}
			view.Log = logStr
		}
		targets = append(targets, view)
	}
	return jobView{
		ID:            job.ID,
		Repo:          job.Repo,
		Commit:        job.Commit,
		TestCommand:   job.TestCommand,
		Architectures: job.Architectures,
		Status:        job.Status,
		Targets:       targets,
		CreatedAt:     job.CreatedAt,
		UpdatedAt:     job.UpdatedAt,
		StartedAt:     job.StartedAt,
		EndedAt:       job.EndedAt,
	}
}

func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logging.Logger.Error("handler_panic", "error", err, "path", r.URL.Path)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
