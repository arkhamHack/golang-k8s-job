package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/avigyan/k8s-priority-queue/pkg/kubernetes"
	"github.com/avigyan/k8s-priority-queue/pkg/server"
	"sigs.k8s.io/yaml"
)

func main() {
	// Parse command-line flags
	var (
		kubeconfigPath string
		port           int
		maxConcurrency int
	)

	// Set default kubeconfig path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Failed to get user home directory: %v", err)
	}
	defaultKubeconfigPath := filepath.Join(homeDir, ".kube", "config")

	// Define flags
	flag.StringVar(&kubeconfigPath, "kubeconfig", defaultKubeconfigPath, "Path to kubeconfig file")
	flag.IntVar(&port, "port", 8080, "HTTP server port")
	flag.IntVar(&maxConcurrency, "max-concurrency", 5, "Maximum number of concurrent job submissions")
	flag.Parse()

	log.Printf("Using kubeconfig at: %s", kubeconfigPath)
	log.Printf("Starting server on port: %d", port)
	log.Printf("Max concurrency: %d", maxConcurrency)

	// Initialize Kubernetes client
	kubeClient, err := kubernetes.NewKubeClient(kubeconfigPath)
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	// Create and start the server
	srv := server.NewServer(kubeClient, port, maxConcurrency)
	
	// Process any additional arguments as job definition files with priorities
	remaining := flag.Args()
	if len(remaining) > 0 {
		go func() {
			// Wait a moment for the server to start
			time.Sleep(1 * time.Second)
			
			// Each pair of arguments should be (jobfile, priority)
			for i := 0; i < len(remaining); i += 2 {
				if i+1 >= len(remaining) {
					log.Printf("Warning: Job file %s provided without priority, skipping", remaining[i])
					break
				}
				
				jobFile := remaining[i]
				priority, err := strconv.Atoi(remaining[i+1])
				if err != nil {
					log.Printf("Invalid priority for job %s: %v, skipping", jobFile, err)
					continue
				}
				
				log.Printf("Processing job file %s with priority %d", jobFile, priority)
				
				// Read the job file
				jobData, err := os.ReadFile(jobFile)
				if err != nil {
					log.Printf("Failed to read job file %s: %v", jobFile, err)
					continue
				}
				
				// Parse file extension to handle YAML if needed
				var jobSpec map[string]interface{}
				if strings.HasSuffix(strings.ToLower(jobFile), ".yaml") || strings.HasSuffix(strings.ToLower(jobFile), ".yml") {
					// Convert YAML to JSON
					var jsonData []byte
					jsonData, err = yaml.YAMLToJSON(jobData)
					if err != nil {
						log.Printf("Failed to convert YAML to JSON for file %s: %v", jobFile, err)
						continue
					}
					jobData = jsonData
				}
				
				// Extract job name from filename if not specified
				jobName := filepath.Base(jobFile)
				jobName = strings.TrimSuffix(jobName, filepath.Ext(jobName))
				
				// Create job submission request
				request := struct {
					Name      string                 `json:"name"`
					Priority  int                    `json:"priority"`
					Namespace string                 `json:"namespace"`
					JobSpec   map[string]interface{} `json:"jobSpec"`
				}{
					Name:      jobName,
					Priority:  priority,
					Namespace: "default",
				}
				
				// Parse the job definition
				if err := json.Unmarshal(jobData, &jobSpec); err != nil {
					log.Printf("Failed to parse job file %s: %v", jobFile, err)
					continue
				}
				request.JobSpec = jobSpec
				
				// Submit the job to the API
				jobJSON, _ := json.Marshal(request)
				resp, err := http.Post(fmt.Sprintf("http://localhost:%d/jobs", port), "application/json", bytes.NewBuffer(jobJSON))
				if err != nil {
					log.Printf("Failed to submit job %s: %v", jobFile, err)
					continue
				}
				defer resp.Body.Close()
				
				// Check response
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					log.Printf("Successfully submitted job %s with priority %d", jobName, priority)
				} else {
					log.Printf("Failed to submit job %s: HTTP %d", jobName, resp.StatusCode)
					respBody, _ := io.ReadAll(resp.Body)
					log.Printf("Response: %s", string(respBody))
				}
			}
		}()
	}

	// Setup graceful shutdown
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGTERM)

	// Start the server in a goroutine
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Println("Server started successfully")

	// Wait for termination signal
	<-stopCh
	log.Println("Received termination signal")

	// Create a context with a timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Attempt graceful shutdown
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server exited gracefully")
}
