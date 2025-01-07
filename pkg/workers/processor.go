package workers

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/arnavsurve/promise/pkg/db"
	"github.com/arnavsurve/promise/pkg/models"
)

const maxRetries = 3

// InitWorkerPool initializes a pool of workers to execute jobs asynchronously
func InitWorkerPool(store *db.Store, numWorkers int, retryCount int) {
	ctx := context.Background()
	jobChannel := make(chan string, 100) // Buffered channel for job commands

	// Start worker goroutines
	fmt.Printf("Initializing worker pool of size: %d\n", numWorkers)
	for i := 0; i < numWorkers; i++ {
		go worker(ctx, store, jobChannel, i, retryCount)
	}

	// Main loop to fetch from Redis and distribute to workers
	for {
		result, err := store.Rdb.BLPop(ctx, 0, "job_queue").Result()
		if err != nil {
			log.Printf("Failed to fetch job from Redis: %s\n", err)
			continue
		}

		command := result[1]
		log.Printf("Dequeued job: %s\n", command)

		// Push command to the job channel
		jobChannel <- command
	}
}

// worker processes jobs received from a channel
func worker(ctx context.Context, store *db.Store, jobChannel chan string, workerId int, maxRetries int) {
	for command := range jobChannel {
		log.Printf("Worker %d processing job: %s\n", workerId, command)

		// Fetch or initialize job
		var job models.Job
		if err := store.DB.Where("command = ? AND status IN ?", command, []string{"Pending", "Retrying"}).First(&job).Error; err != nil {
			// If job not found, create a new job
			job = models.Job{
				Command:       command,
				Status:        "Running",
				ExecutionTime: time.Now().UTC(),
				RetryCount:    0,
			}
			if err := store.DB.Create(&job).Error; err != nil {
				log.Printf("Worker %d failed to log job in database: %s\n", workerId, err)
				continue
			}
		} else {
			// Mark job as running if retrying
			job.Status = "Running"
			job.ExecutionTime = time.Now().UTC()
			if err := store.DB.Save(&job).Error; err != nil {
				log.Printf("Worker %d failed to update job to 'Running': %s\n", workerId, err)
				continue
			}
		}

		// Execute the command
		err := executeCommand(command)
		if err != nil {
			log.Printf("Worker %d job failed: %s\n", workerId, err)

			// Retry if allowed
			if job.RetryCount < maxRetries-1 {
				job.RetryCount++
				job.Status = "Retrying"
				job.ExecutionTime = time.Now().UTC()
				if err := store.DB.Save(&job).Error; err != nil {
					log.Printf("Worker %d failed to update retry count for job %d: %s\n", workerId, job.ID, err)
					continue
				}

				// Requeue job after a delay
				go func(cmd string) {
					log.Printf("Worker %d retrying job %d in 5 seconds ... (Retry %d/%d)\n\n", workerId, job.ID, job.RetryCount, maxRetries)
					time.Sleep(5 * time.Second)
					if retryErr := store.Rdb.RPush(ctx, "job_queue", cmd).Err(); retryErr != nil {
						log.Printf("Worker %d failed to requeue job %s: %s\n", workerId, cmd, retryErr)
					}
				}(command)
			} else {
				// Mark job as failed if max retries reached
				job.Status = "Failed"
				if err := store.DB.Save(&job).Error; err != nil {
					log.Printf("Worker %d failed to mark job %d as failed: %s\n\n", workerId, job.ID, err)
				} else {
					log.Printf("Worker %d reached max (%d/%d) retries for job %d: %s. Marking as failed.\n\n", workerId, job.RetryCount, maxRetries, job.ID, command)
				}
			}
		} else {
			// Mark job as completed
			job.Status = "Completed"
			if err := store.DB.Save(&job).Error; err != nil {
				log.Printf("Worker %d failed to mark job %d as completed: %s\n\n", workerId, job.ID, err)
			} else {
				log.Printf("Worker %d successfully completed job %d: %s\n\n", workerId, job.ID, command)
			}
		}
	}
}

// executeCommand runs a shell command and returns the result
func executeCommand(command string) error {
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	log.Printf("Command output: \n%s", string(output))
	return err
}
