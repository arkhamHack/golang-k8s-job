#!/usr/bin/env python3
import requests
import json
import time
import random
import argparse
from datetime import datetime

def submit_job(base_url, name, priority, sleep_seconds=300):  # Default 5 minutes
    """Submit a job with given priority and much longer sleep time"""
    url = f"{base_url}/jobs"
    
    job_spec = {
        "name": name,
        "priority": priority,
        "namespace": "default",
        "jobSpec": {
            "apiVersion": "batch/v1",
            "kind": "Job",
            "metadata": {
                "name": name
            },
            "spec": {
                # Make sure jobs run longer
                "activeDeadlineSeconds": sleep_seconds + 30,  # Slightly longer than sleep time
                "template": {
                    "spec": {
                        "containers": [
                            {
                                "name": "test",
                                "image": "busybox",
                                "command": ["sh", "-c", f"echo Starting sleep for {sleep_seconds}s; sleep {sleep_seconds}; echo Sleep complete"]
                            }
                        ],
                        "restartPolicy": "Never"
                    }
                }
            }
        }
    }
    
    try:
        response = requests.post(url, json=job_spec)
        data = response.json()
        
        # If we got a valid response with an ID, consider it successful
        if response.status_code == 202 and 'id' in data:
            print(f"Submitted job: {name} with priority {priority} - Queue Position: {data.get('queuePosition')}")
            return data
        else:
            print(f"Failed to submit job {name} - Status: {response.status_code}")
            return None
    except Exception as e:
        print(f"Error submitting job {name}: {str(e)}")
        return None

def get_pending_jobs(base_url):
    """Get list of pending jobs"""
    response = requests.get(f"{base_url}/jobs/pending")
    if response.status_code == 200:
        return response.json()
    else:
        print(f"Failed to get pending jobs: {response.text}")
        return []

def get_running_jobs(base_url):
    """Get list of running jobs"""
    response = requests.get(f"{base_url}/jobs/running")
    if response.status_code == 200:
        return response.json()
    else:
        print(f"Failed to get running jobs: {response.text}")
        return []

def print_status(pending, running):
    """Print current status of pending and running jobs"""
    print("\n" + "="*80)
    print(f"STATUS AT: {datetime.now().strftime('%H:%M:%S')}")
    print("="*80)
    
    print("\nPENDING JOBS:")
    if pending:
        # Sort by priority (highest first)
        sorted_pending = sorted(pending, key=lambda x: x.get('priority', 0), reverse=True)
        for job in sorted_pending:
            print(f"  - {job.get('name')}: Priority={job.get('priority')}")
    else:
        print("  No pending jobs")
    
    print("\nRUNNING JOBS:")
    if running:
        for job in running:
            print(f"  - {job.get('name')} (Namespace: {job.get('namespace')})")
    else:
        print("  No running jobs")
    
    print(f"\nCounts: {len(pending)} pending, {len(running)} running")
    print("="*80 + "\n")

def run_test(base_url, num_jobs=10, max_priority=100, monitor_seconds=300, concurrency=3):
    print(f"\nTesting service at {base_url}\n")
    print(f"Expected worker concurrency: {concurrency}")
    
    print("Submitting jobs with random priorities and LONG execution times...")
    submitted_jobs = []
    
    for i in range(1, num_jobs + 1):
        priority = random.randint(1, max_priority)
        
        sleep_seconds = 20 + (i * 1)  # 20-30 seconds
        
        job_name = f"test-p{priority:03d}-{int(time.time())}-{i}"  # Include priority in name
        job = submit_job(base_url, job_name, priority, sleep_seconds)
        if job:
            submitted_jobs.append({
                "name": job_name,
                "priority": priority,
                "id": job.get("id")
            })
        # Brief pause between submissions to ensure stable queue state
        time.sleep(1)
    
    print("\nAll jobs submitted. Monitoring for", monitor_seconds, "seconds...\n")
    
    # Monitor pending and running jobs
    end_time = time.time() + monitor_seconds
    while time.time() < end_time:
        pending = get_pending_jobs(base_url)
        running = get_running_jobs(base_url)
        print_status(pending, running)
        
        # Validate priority processing
        if pending:
            priorities = [job.get('priority', 0) for job in pending]
        
        # Validate concurrency
        if len(running) > concurrency:
            print(f"WARNING: Running {len(running)} jobs, expected max {concurrency}!")
            
        time.sleep(10)  # Check every 10 seconds
    
    print("\nTest completed!")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description='Test Kubernetes Job Priority Queue')
    parser.add_argument('--url', default='http://localhost:8080', help='Base URL of the service')
    parser.add_argument('--jobs', type=int, default=8, help='Number of jobs to submit')
    parser.add_argument('--monitor', type=int, default=30, help='Seconds to monitor job status')
    parser.add_argument('--concurrency', type=int, default=3, help='Expected concurrency limit')
    args = parser.parse_args()
    
    run_test(args.url, args.jobs, monitor_seconds=args.monitor, concurrency=args.concurrency)