package kubernetes

import (
	"context"
	"fmt"
	"log"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type KubeClient struct {
	clientset *kubernetes.Clientset
}

func NewKubeClient(kubeconfigPath string) (*KubeClient, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %v", err)
	}

	return &KubeClient{
		clientset: clientset,
	}, nil
}

func (k *KubeClient) SubmitJob(ctx context.Context, job *batchv1.Job) (*batchv1.Job, error) {
	namespace := job.Namespace
	if namespace == "" {
		namespace = "default"
	}

	result, err := k.clientset.BatchV1().Jobs(namespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create job: %v", err)
	}

	log.Printf("Successfully created job %s in namespace %s", result.Name, namespace)
	return result, nil
}

func (k *KubeClient) ListRunningJobs(ctx context.Context, namespace string) ([]batchv1.Job, error) {
	if namespace == "" {
		namespace = "default"
	}

	jobs, err := k.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %v", err)
	}

	runningJobs := make([]batchv1.Job, 0)
	for _, job := range jobs.Items {
		if job.Status.Active > 0 || (job.Status.StartTime != nil && job.Status.CompletionTime == nil) {
			runningJobs = append(runningJobs, job)
		}
	}

	return runningJobs, nil
}
