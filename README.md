# Kubernetes Priority Queue

This is a Go-based HTTP API server that accepts job submissions with priorities, enqueues them in a max-priority queue, and submits them to a Kubernetes cluster with controlled concurrency.

Task 2 Write Up is in the containerd-snapshotters.md file.

## Features

- Loads kubeconfig file and initializes a Kubernetes client
- Accepts job submissions via POST endpoints with associated numeric priorities
- Enqueues jobs in a max-priority queue using Go's container/heap
- Controls job submission concurrency via configurable limits
- Submits each job as a batch/v1.Job to the Kubernetes cluster
- Provides endpoints for inspecting pending and running jobs



## Build Instructions

```bash
# Clone the repository
git clone https://github.com/avigyan/k8s-priority-queue.git
cd k8s-priority-queue

# Download dependencies
go mod tidy

# Build the application
go build -o k8s-priority-queue ./cmd/main.go
```

## Usage

### Running as a Server

```bash
# Run with default settings (kubeconfig at ~/.kube/config, port 8080, max concurrency 5)
./k8s-priority-queue

# Run with custom settings
./k8s-priority-queue -kubeconfig=/path/to/kubeconfig -port=8080 -max-concurrency=10

# With a local kind cluster
# First ensure your kubeconfig is properly configured for kind
kind get kubeconfig > kind-config
./k8s-priority-queue -kubeconfig=./kind-config -max-concurrency=3
```

### Submitting Jobs from Files

You can submit one or more job definition files directly from the command line as arguments after the flags. Each job file must be followed by its priority:

```bash

./k8s-priority-queue -kubeconfig=./kind-config -max-concurrency=2 job1.json 50
```
Can also use test.py for sample testing:
```bash
python test.py --jobs 8 --concurrency 3 --monitor 60
```

### Command-line Flags

| Flag | Description | Default |
|------|-------------|--------|
| `-kubeconfig` | Path to kubeconfig file | `~/.kube/config` |
| `-port` | HTTP server port | `8080` |
| `-max-concurrency` | Maximum number of concurrent job submissions | `5` |


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

2. Check pending jobs to see them ordered by priority:
   ```
   curl http://localhost:8080/jobs/pending
   ```

3. Check running jobs:
   ```
   curl http://localhost:8080/jobs/running
   ```

## Graceful Shutdown

The server supports graceful shutdown, allowing in-flight jobs to complete before exiting. Press Ctrl+C to initiate a graceful shutdown.

## Example Job Files

### JSON Example (job1.json)

```json
{
  "apiVersion": "batch/v1",
  "kind": "Job",
  "metadata": {
    "name": "example-job"
  },
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "hello",
            "image": "busybox",
            "command": ["echo", "Hello from priority queue!"]
          }
        ],
        "restartPolicy": "Never"
      }
    },
    "backoffLimit": 0
  }
}
```

### YAML Example (job2.yaml)

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: yaml-example-job
spec:
  template:
    spec:
      containers:
      - name: sleep
        image: busybox
        command: ["sleep", "10"]
      restartPolicy: Never
  backoffLimit: 0
```
