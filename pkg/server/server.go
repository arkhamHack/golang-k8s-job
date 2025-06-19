package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"container/heap"

	batchv1 "k8s.io/api/batch/v1"

	"github.com/google/uuid"

	"github.com/avigyan/k8s-priority-queue/pkg/kubernetes"
	"github.com/avigyan/k8s-priority-queue/pkg/queue"
)

type JobSpec struct {
	Name      string      `json:"name"`
	Priority  int         `json:"priority"`
	Namespace string      `json:"namespace"`
	JobSpec   interface{} `json:"jobSpec"`
}

type JobResponse struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Priority      int    `json:"priority"`
	QueuePosition int    `json:"queuePosition"`
}

type PendingJobResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Priority int    `json:"priority"`
}

type Server struct {
	kubeClient       *kubernetes.KubeClient
	priorityQueue    *queue.MaxPriorityQueue
	enqueueCh        chan *queue.Job
	maxConcurrency   int
	runningJobsMutex sync.RWMutex
	runningJobs      map[string]*queue.Job
	httpServer       *http.Server
	shutdownCh       chan struct{}
	wg               sync.WaitGroup
}

func NewServer(kubeClient *kubernetes.KubeClient, port int, maxConcurrency int) *Server {
	s := &Server{
		kubeClient:     kubeClient,
		priorityQueue:  queue.NewPriorityQueue(),
		enqueueCh:      make(chan *queue.Job),
		maxConcurrency: maxConcurrency,
		runningJobs:    make(map[string]*queue.Job),
		shutdownCh:     make(chan struct{}),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", s.handleJobs)
	mux.HandleFunc("/jobs/pending", s.handlePendingJobs)
	mux.HandleFunc("/jobs/running", s.handleRunningJobs)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	return s
}

func (s *Server) Start() error {
	for i := 0; i < s.maxConcurrency; i++ {
		s.wg.Add(1)
		go s.worker()
	}

	s.wg.Add(1)
	go s.queueProcessor()

	log.Printf("Starting HTTP server on %s", s.httpServer.Addr)
	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("http server error: %v", err)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")

	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("error shutting down HTTP server: %v", err)
	}

	close(s.shutdownCh)

	log.Println("Waiting for workers to complete...")
	s.wg.Wait()

	log.Println("Server shutdown complete")
	return nil
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var jobSpec JobSpec
	if err := json.NewDecoder(r.Body).Decode(&jobSpec); err != nil {
		http.Error(w, fmt.Sprintf("Error decoding request: %v", err), http.StatusBadRequest)
		return
	}

	job := &queue.Job{
		ID:       uuid.New().String(),
		Name:     jobSpec.Name,
		Priority: jobSpec.Priority,
		Spec:     jobSpec,
	}

	position := s.priorityQueue.GetPositionByPriority(job.Priority)

	heap.Push(s.priorityQueue, job)
	log.Printf("Job %s with priority %d added to queue at position %d", job.Name, job.Priority, position)

	response := JobResponse{
		ID:            job.ID,
		Name:          job.Name,
		Priority:      job.Priority,
		QueuePosition: position,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handlePendingJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("Handling request for pending jobs")
	pendingJobs := s.priorityQueue.PendingJobs()
	log.Printf("Found %d pending jobs", len(pendingJobs))

	response := make([]map[string]interface{}, len(pendingJobs))
	for i, job := range pendingJobs {
		log.Printf("Pending job %d: %s (priority %d)", i+1, job.Name, job.Priority)
		response[i] = map[string]interface{}{
			"id":       job.ID,
			"name":     job.Name,
			"priority": job.Priority,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *Server) handleRunningJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Query Kubernetes API for running jobs
	ctx := context.Background()
	k8sRunningJobs, err := s.kubeClient.ListRunningJobs(ctx, "") // Empty string for all namespaces
	if err != nil {
		log.Printf("Error listing running jobs from Kubernetes: %v", err)
		http.Error(w, "Failed to list running jobs", http.StatusInternalServerError)
		return
	}

	// Build response with job info
	runningJobs := make([]map[string]interface{}, 0, len(k8sRunningJobs))
	for _, k8sJob := range k8sRunningJobs {
		jobInfo := map[string]interface{}{
			"name":      k8sJob.Name,
			"namespace": k8sJob.Namespace,
		}

		// Try to extract our job ID from labels if it exists
		if jobID, ok := k8sJob.Labels["job-id"]; ok {
			jobInfo["id"] = jobID
		}
		if priority, ok := k8sJob.Labels["priority"]; ok {
			if p, err := strconv.Atoi(priority); err == nil {
				jobInfo["priority"] = p
			}
		}

		runningJobs = append(runningJobs, jobInfo)
	}

	log.Printf("Found %d running jobs in Kubernetes", len(runningJobs))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runningJobs)
}

func (s *Server) queueProcessor() {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdownCh:
			log.Println("Queue processor shutting down...")
			return
		default:
			// Check if we can process more jobs
			ctx := context.Background()
			runningK8sJobs, err := s.kubeClient.ListRunningJobs(ctx, "")
			if err != nil {
				log.Printf("Error listing running jobs: %v. Will retry.", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			// Only process jobs if below max concurrency
			if len(runningK8sJobs) >= s.maxConcurrency {
				log.Printf("Max concurrency (%d) reached with %d running jobs. Waiting...",
					s.maxConcurrency, len(runningK8sJobs))
				time.Sleep(2 * time.Second)
				continue
			}

			// Only proceed if there are jobs in the queue
			if s.priorityQueue.Len() == 0 {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Get highest priority job
			job := heap.Pop(s.priorityQueue)
			if job == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			jobData := job.(*queue.Job)
			log.Printf("Queue processor dispatching job %s with priority %d", jobData.Name, jobData.Priority)

			s.runningJobsMutex.Lock()
			s.runningJobs[jobData.ID] = jobData
			s.runningJobsMutex.Unlock()

			go s.processJob(jobData)

			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (s *Server) worker() {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdownCh:
			log.Println("Worker shutting down...")
			return
		default:
			// Check current concurrency against Kubernetes, not just in-memory map
			ctx := context.Background()
			runningK8sJobs, err := s.kubeClient.ListRunningJobs(ctx, "")
			if err != nil {
				log.Printf("Error listing running jobs: %v. Will retry.", err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			if len(runningK8sJobs) >= s.maxConcurrency {
				log.Printf("Max concurrency (%d) reached with %d running jobs. Waiting...",
					s.maxConcurrency, len(runningK8sJobs))
				time.Sleep(2 * time.Second)
				continue
			}

			if s.priorityQueue.Len() == 0 {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			job := heap.Pop(s.priorityQueue)

			if job == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			jobData := job.(*queue.Job)
			log.Printf("Worker processing job %s with priority %d", jobData.Name, jobData.Priority)

			s.runningJobsMutex.Lock()
			s.runningJobs[jobData.ID] = jobData
			s.runningJobsMutex.Unlock()

			go s.processJob(jobData)
		}
	}
}

func (s *Server) processJob(j *queue.Job) {
	ctx := context.Background()
	jobSpec := j.Spec.(JobSpec)

	var k8sJob *batchv1.Job
	jobSpecBytes, err := json.Marshal(jobSpec.JobSpec)
	if err != nil {
		log.Printf("Error marshaling job spec: %v", err)
		return
	}

	if err := json.Unmarshal(jobSpecBytes, &k8sJob); err != nil {
		log.Printf("Error unmarshaling job spec to batch/v1.Job: %v", err)
		return
	}

	if k8sJob.Namespace == "" {
		k8sJob.Namespace = jobSpec.Namespace
	}
	if k8sJob.Namespace == "" {
		k8sJob.Namespace = "default"
	}

	result, err := s.kubeClient.SubmitJob(ctx, k8sJob)
	if err != nil {
		log.Printf("Error submitting job %s: %v", j.Name, err)
		return
	}

	log.Printf("Job %s submitted successfully. Kubernetes job name: %s", j.Name, result.Name)

	log.Printf("Job %s submitted, will monitor for completion", j.Name)

	for {
		time.Sleep(5 * time.Second)

		jobs, err := s.kubeClient.ListRunningJobs(ctx, k8sJob.Namespace)
		if err != nil {
			log.Printf("Error checking job %s status: %v", j.Name, err)
			continue
		}

		jobStillRunning := false
		for _, runningJob := range jobs {
			if runningJob.Name == result.Name && runningJob.Namespace == result.Namespace {
				jobStillRunning = true
				break
			}
		}

		if !jobStillRunning {
			log.Printf("Job %s completed or failed in Kubernetes", j.Name)
			s.runningJobsMutex.Lock()
			delete(s.runningJobs, j.ID)
			s.runningJobsMutex.Unlock()

			log.Printf("Removed job %s from tracking map", j.Name)
			break
		}
	}
}
