package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/avigyan/k8s-priority-queue/pkg/kubernetes"
	"github.com/avigyan/k8s-priority-queue/pkg/server"
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
