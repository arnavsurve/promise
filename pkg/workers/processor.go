package workers

import (
	"log"
	"os/exec"
	"time"

	"github.com/arnavsurve/promise/pkg/db"
	"github.com/arnavsurve/promise/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ProcessJobs(s *db.Store) {
	for {
		var job models.Job

		// Fetch next queued job
		err := s.DB.Transaction(func(tx *gorm.DB) error {
			// Fetch the first job, avoiding blocking on already locked jobs
			err := tx.Where("status = ?", "Queued").
				Order("created_at").
				Clauses(clause.Locking{Strength: "UPDATE SKIP LOCKED"}).
				First(&job).Error
			if err != nil {
				if err == gorm.ErrRecordNotFound {
					log.Println("No jobs found. Sleeping... 😴")
					time.Sleep(5 * time.Second)
					return nil
				}
				return err
			}

			// Ensure a valid job was fetched
			// if job.ID == 0 {
			// 	log.Println("No valid jobs fetched. Skipping...")
			// 	return nil
			// }

			// Mark job as running
			if err := tx.Model(&models.Job{}).Where("id = ?", job.ID).Update("status", "Running").Error; err != nil {
				return err
			}

			return nil
		})
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

		if err := s.DB.Model(&job).Update("status", status).Error; err != nil {
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
	log.Printf("Command output: %s\n", string(output))
	return err
}
