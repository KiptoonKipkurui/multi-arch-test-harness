# Multi-Arch Distributed Test Harness

A local-first test harness that runs your test suite across multiple CPU architectures (for example amd64 and arm64) behind a single CI job. It provisions ephemeral container environments, executes tests per architecture, and aggregates results through a simple HTTP API for use in GitHub Actions or other CI systems.

## Why this exists

Modern software increasingly needs to support multiple architectures: x86 servers, ARM servers, edge devices, and Apple Silicon developer machines. Traditional CI pipelines either:

- Only test on one architecture and risk architecture-specific bugs, or
- Duplicate pipeline logic per architecture, which complicates configuration and maintenance.

This project provides a single, consistent entry point for multi-arch testing:

- CI calls one API endpoint.
- The harness provisions separate environments per architecture (locally via Docker in v1).
- Tests run in each environment, and the harness aggregates the results and exposes a unified status.

The goal is to model the kind of distributed, multi-arch testing infrastructure used by teams working on operating systems, containers, and cloud platforms.

## High-level architecture

- **Control Plane (Go)**
  - Exposes a REST API (`/jobs`, `/jobs/{id}`) to trigger and inspect test runs.
  - Schedules per-architecture targets for each job.
  - Uses a local Docker-based provisioner to start ephemeral test environments.
  - Receives results from agents and stores job state.

- **Provisioner (local, Docker-based)**
  - Spins up containers using architecture-specific or multi-arch images (with Docker buildx and emulation where needed).
  - Cleans up containers when tests complete.

- **Agent (containerized runner)**
  - Runs inside each provisioned container.
  - Checks out the specified repository and commit.
  - Executes the configured test command (for example `go test ./...`).
  - Sends exit code, logs, and basic timing information back to the control plane.

- **CI Integration (GitHub Actions)**
  - A GitHub Actions workflow calls the control plane on each push/PR.
  - CI waits for the job to complete and fails if any architecture fails.

## Features (v1)

- Trigger a multi-arch test run via a simple HTTP API.
- Run tests locally in Docker containers for:
  - `amd64` (native on typical dev machines).
  - `arm64` (via multi-arch images and emulation).
- Aggregate per-architecture results under a single job ID.
- Inspect job status and per-architecture results via `GET /jobs/{id}`.
- Integrate with GitHub Actions to enforce “all architectures must pass” on pull requests.

Planned extensions include better persistence, richer metrics, and additional provisioners (LXD, local Kubernetes, or real cloud providers).

## Getting started

### Prerequisites

- Go (recent stable version).
- Docker with buildx and multi-arch support enabled.
- Git.

### Run the control plane
``` bash
git clone https://github.com/your-user/multiarch-test-harness.git
cd multiarch-test-harness

go run ./cmd/server 
```



By default, the server listens on `http://localhost:8080` (configurable via environment variables).

### Trigger a job manually
``` bash
curl -X POST http://localhost:8080/jobs
-H "Content-Type: application/json"
-d '{
"repo": "https://github.com/your-user/sample-app.git",
"commit": "HEAD",
"test_command": "go test ./...",
"architectures": ["amd64", "arm64"]
}'


```
The response includes a `job_id`. Use it to check the status:

``` bash
curl http://localhost:8080/jobs/<job_id>

```
You will see per-architecture statuses and basic result information.

## GitHub Actions integration

A minimal workflow example:

``` yaml

name: Multi-Arch Tests

on:
pull_request:
push:
branches: [ main ]

jobs:
multiarch-tests:
runs-on: ubuntu-latest
steps:
- name: Trigger multi-arch test job
run: |
JOB_ID=$(curl -s -X POST http://your-server:8080/jobs
-H "Content-Type: application/json"
-d "{
"repo": "https://github.com/${{ github.repository }}.git",
"commit": "${{ github.sha }}",
"test_command": "go test ./...",
"architectures": ["amd64", "arm64"]
}" | jq -r '.job_id')


      for i in {1..60}; do
        STATUS=$(curl -s http://your-server:8080/jobs/$JOB_ID | jq -r '.status')
        echo "Job status: $STATUS"
        if [ "$STATUS" = "passed" ]; then exit 0; fi
        if [ "$STATUS" = "failed" ]; then exit 1; fi
        sleep 10
      done

      echo "Timed out waiting for job"
      exit 1


```

In local development, you can run the server on your machine and point the workflow to it (or run the server in a self-hosted runner).

## Roadmap

- Persistent storage (sqlite/Postgres) for jobs and results.
- Retry policies and flaky test detection.
- Richer logs and metrics endpoints (for example for Prometheus/Grafana).
- Additional provisioners (LXD, local Kubernetes, or real cloud/bare-metal environments).
- Optional web UI for browsing runs and drilling down into per-architecture results.

## Why this matters

This project demonstrates how to:

- Design and implement a small but realistic distributed testing platform in Go.
- Work with containers and multi-architecture images in CI workflows.
- Separate concerns between CI, environment provisioning, and test execution, which is central to robust cloud and distributed systems testing.

It is intentionally local-first and simple to run, making it a practical tool for day-to-day development and a strong portfolio piece for roles focused on containers, distributed systems, and CI tooling.
