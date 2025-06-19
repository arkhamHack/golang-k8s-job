# Kubernetes Priority Queue

This is a Go-based HTTP API server that accepts job submissions with priorities, enqueues them in a max-priority queue, and submits them to a Kubernetes cluster with controlled concurrency.

## Features

- Loads kubeconfig file and initializes a Kubernetes client
- Accepts job submissions via POST endpoints with associated numeric priorities
- Enqueues jobs in a max-priority queue using Go's container/heap
- Controls job submission concurrency via configurable limits
- Submits each job as a batch/v1.Job to the Kubernetes cluster
- Provides endpoints for inspecting pending and running jobs
- Supports graceful shutdown to drain in-flight work

## Requirements

- Go 1.18 or later
- Access to a Kubernetes cluster with a valid kubeconfig file

## Build Instructions

```bash
# Clone the repository
git clone <repository-url>
cd k8s-priority-queue

# Download dependencies
go mod tidy

# Build the application
go build -o k8s-priority-queue ./cmd
```

## Usage

```bash
# Run with default settings (kubeconfig at ~/.kube/config, port 8080, max concurrency 5)
./k8s-priority-queue

# Run with custom settings
./k8s-priority-queue -kubeconfig=/path/to/kubeconfig -port=8080 -max-concurrency=10
```

## API Endpoints

### Submit a Job (POST /jobs)

```bash
curl -X POST http://localhost:8080/jobs \
  -H "Content-Type: application/json" \
  -d @job-spec.json
```

Example `job-spec.json`:

```json
{
  "name": "sleep-job",
  "priority": 10,
  "namespace": "default",
  "jobSpec": {
    "apiVersion": "batch/v1",
    "kind": "Job",
    "metadata": {
      "generateName": "sleep-job-"
    },
    "spec": {
      "template": {
        "spec": {
          "containers": [
            {
              "name": "sleep",
              "image": "busybox",
              "command": ["sleep", "30"]
            }
          ],
          "restartPolicy": "Never"
        }
      },
      "backoffLimit": 0
    }
  }
}
```

### List Pending Jobs (GET /jobs/pending)

```bash
curl http://localhost:8080/jobs/pending
```

### List Running Jobs (GET /jobs/running)

```bash
curl http://localhost:8080/jobs/running
```

## Example Usage

1. Start the server:
   ```
   ./k8s-priority-queue
   ```

2. Submit multiple jobs with different priorities:
   ```
   curl -X POST http://localhost:8080/jobs -H "Content-Type: application/json" -d @high-priority.json
   curl -X POST http://localhost:8080/jobs -H "Content-Type: application/json" -d @medium-priority.json
   curl -X POST http://localhost:8080/jobs -H "Content-Type: application/json" -d @low-priority.json
   ```

3. Check pending jobs to see them ordered by priority:
   ```
   curl http://localhost:8080/jobs/pending
   ```

4. Check running jobs:
   ```
   curl http://localhost:8080/jobs/running
   ```

## Graceful Shutdown

The server supports graceful shutdown, allowing in-flight jobs to complete before exiting. Press Ctrl+C to initiate a graceful shutdown.
