package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"container/heap"

	"github.com/avigyan/k8s-priority-queue/pkg/kubernetes"
	"github.com/avigyan/k8s-priority-queue/pkg/queue"
	"github.com/google/uuid"
	batchv1 "k8s.io/api/batch/v1"
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

	s.enqueueCh <- job

	response := JobResponse{
		ID:            job.ID,
		Name:          job.Name,
		Priority:      job.Priority,
		QueuePosition: s.priorityQueue.Len() + 1,
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

	pendingJobs := s.priorityQueue.PendingJobs()
	response := make([]PendingJobResponse, len(pendingJobs))

	for i, job := range pendingJobs {
		response[i] = PendingJobResponse{
			ID:       job.ID,
			Name:     job.Name,
			Priority: job.Priority,
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

	// Use the in-memory runningJobs map to show only jobs currently being processed by workers
	s.runningJobsMutex.RLock()
	defer s.runningJobsMutex.RUnlock()

	runningJobs := make([]map[string]interface{}, 0, len(s.runningJobs))
	for _, job := range s.runningJobs {
		jobInfo := map[string]interface{}{
			"id":       job.ID,
			"name":     job.Name,
			"priority": job.Priority,
		}

		if jobSpec, ok := job.Spec.(JobSpec); ok {
			jobInfo["namespace"] = jobSpec.Namespace
		}

		runningJobs = append(runningJobs, jobInfo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runningJobs)
}

func (s *Server) queueProcessor() {
	defer s.wg.Done()

	for {
		select {
		case job := <-s.enqueueCh:
			log.Printf("Enqueueing job %s with priority %d", job.Name, job.Priority)
			heap.Push(s.priorityQueue, job)
		case <-s.shutdownCh:
			log.Println("Queue processor shutting down...")
			return
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

			ctx := context.Background()
			func(j *queue.Job) {
				defer func() {
					s.runningJobsMutex.Lock()
					delete(s.runningJobs, j.ID)
					s.runningJobsMutex.Unlock()
				}()

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
			}(jobData)
		}
	}
}
