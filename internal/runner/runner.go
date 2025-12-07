package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/config"
	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/core"
	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/logging"
	"github.com/kiptoonkipkurui/multi-arch-test-harness/internal/store"
)

type Runner struct {
	store  store.Store
	config *config.Config
}

func NewRunner(st store.Store) *Runner {
	return &Runner{
		store:  st,
		config: config.Load(),
	}
}

// RunJobAsync starts running goroutines for each target architecture
func (r *Runner) RunJobAsync(job *core.Job) {
	for _, target := range job.Targets {
		go r.runTarget(job.ID, job, target.Arch)
	}
}

func (r *Runner) runTarget(jobID string, job *core.Job, arch string) {
	now := time.Now()
	// Mark target as running
	r.store.UpdateTarget(jobID, arch, func(j *core.Job, t *core.JobTarget) {
		t.Status = core.TargetStatusRunning
		t.StartedAt = &now
	})
	r.store.RecalculateJobStatus(jobID)
	logging.Logger.Info("target_start",
		"job_id", jobID,
		"arch", arch,
		"phase", "provision",
	)

	// Build docker image name/tag for this arch
	image := fmt.Sprintf("multi-arch-test-runner:%s", arch)

	// cmd: docker run --rm -t IMAGE sh -c "git clone REPO app && cd app && <test_command>"
	testCmd := fmt.Sprintf("git clone %s app && cd app && %s", job.Repo, job.TestCommand)

	// configurable timeout
	timeoutStr := job.Timeout
	if timeoutStr == "" {
		timeoutStr = fmt.Sprintf("%v", r.config.DefaultTimeout)
	}
	timeout, _ := time.ParseDuration(timeoutStr)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Docker args with env vars
	dockerArgs := []string{"run", "--rm", "-t", image, "sh", "-c", testCmd}

	for k, v := range job.Env {
		dockerArgs = append(dockerArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	var stdout, stderr bytes.Buffer
	logging.Logger.Info("target_phase",
		"job_id", jobID,
		"arch", arch,
		"phase", "docker_run",
		"image", image,
	)
	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	// Classify errors
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	reason := ""

	if ctx.Err() == context.DeadlineExceeded {
		reason = "timeout"
		exitCode = -2
	} else if err != nil {
		exitMsg := err.Error()

		switch {
		case strings.Contains(exitMsg, "unable to find image"):
			reason = "docker_image_missing"
		case strings.Contains(exitMsg, "docker daemon"):
			reason = "docker_daemon_error"
		case strings.Contains(stdout.String(), "Username for") ||
			strings.Contains(stderr.String(), "Username for"):
			reason = "git_auth_error"
		default:
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
				if strings.Contains(stdout.String(), "Cloning into") && exitCode != 0 {
					reason = "git_clone_failed"
				} else {
					reason = "tests_failed"
				}
			} else {
				reason = "docker_error"
			}
		}
	} else if exitCode != 0 {
		reason = "tests_failed"
	}
	logBuf := bytes.NewBuffer(nil)
	logBuf.WriteString("STDOUT:\n")
	logBuf.Write(stdout.Bytes())
	logBuf.WriteString("\nSTDERR:\n")
	logBuf.Write(stderr.Bytes())
	status := core.TargetStatusPassed
	if err != nil || exitCode != 0 {
		status = core.TargetStatusFailed
	}

	end := time.Now()
	r.store.UpdateTarget(jobID, arch, func(j *core.Job, t *core.JobTarget) {
		t.Status = status
		t.ExitCode = exitCode
		t.Log = logBuf.String()
		t.EndedAt = &end
		t.Reason = reason
	})
	r.store.RecalculateJobStatus(jobID)
	logging.Logger.Info("target_done",
		"job_id", jobID,
		"arch", arch,
		"phase", "done",
		"status", status,
		"exit_code", exitCode,
	)
}
