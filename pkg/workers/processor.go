package workers

import (
	"context"
	"log"
	"os/exec"

	"github.com/arnavsurve/promise/pkg/db"
	"github.com/arnavsurve/promise/pkg/models"
	"gorm.io/gorm"
	// "gorm.io/gorm"
	// "gorm.io/gorm/clause"
)

func ProcessJobs(store *db.Store) {
	ctx := context.Background()

	for {
		var job models.Job

		// Fetch next queued job
		result, err := store.Rdb.BLPop(ctx, 0, "job_queue").Result()
		if err != nil {
			log.Printf("Failed to fetch job: %s\n", err)
			continue
		}

		// Mark job as running
		err = store.DB.Transaction(func(tx *gorm.DB) error {
			if err = tx.Model(&models.Job{}).Where("id = ?", job.ID).Update("status", "Running").Error; err != nil {
				return err
			}

			return nil
		})
		if err != nil {
			log.Printf("Error during job transaction: %s\n", err)
			continue
		}

		command := result[1]
		log.Printf("Processing job: %s\n", command)

		err = executeCommand(command)
		if err != nil {
			log.Printf("Failed to process job: %s\n", err)
		} else {
			log.Println("Job processed successfully")
		}

		if err != nil {
			log.Printf("Error during job transaction: %s\n", err)
			continue
		}

		if job.ID == 0 {
			continue
		}

		// Execute job
		log.Printf("Processing job %d: %s\n", job.ID, job.Command)
		err = executeCommand(job.Command)

		// Update job status
		status := "Completed"
		if err != nil {
			log.Printf("Job %d failed: %s\n", job.ID, err)
			status = "Failed"
		}

		if err := store.DB.Model(&job).Update("status", status).Error; err != nil {
			log.Printf("Failed to update status for job %d: %s\n", job.ID, err)
		}
	}
}

// executeCommand runs a job, returning the execution status and output
func executeCommand(command string) error {
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("Command execution error: %s\n", err)
	}
	log.Printf("Command output: \n%s\n", string(output))
	return err
}
