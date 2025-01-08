package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/arnavsurve/promise/pkg/db"
	"github.com/arnavsurve/promise/pkg/models"
	"gorm.io/gorm"
)

var ctx = context.Background()

// EnqueueJob adds an incoming task to the queue
func EnqueueJob(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var job models.Job

		// Decode JSON from request body
		err := json.NewDecoder(r.Body).Decode(&job)
		if err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		// Validate that command is not empty
		if job.Command == "" {
			http.Error(w, "Command cannot be empty", http.StatusBadRequest)
			return
		}

		// Set initial status and save to the database
		job.Status = "Queued"
		if err := s.DB.Create(&job).Error; err != nil {
			http.Error(w, "Failed to enqueue job", http.StatusInternalServerError)
			return
		}

		err = PublishJob(s, job.Command)
		if err != nil {
			log.Printf("Error publishing job: %s\n", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Job enqueued successfully",
			"job_id":  job.ID,
		})
	}
}

func PublishJob(s *db.Store, command string) error {
	err := s.Rdb.RPush(ctx, "job_queue", command).Err()
	if err != nil {
		return err
	}
	return nil
}

// GetJobStatus returns a job's status and execution time in UTC by default. Timezone can be defined via URL parameter
func GetJobStatus(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		id := queryParams.Get("id")
		timezone := queryParams.Get("timezone")

		var job models.Job

		if err := s.DB.Where("id = ?", id).First(&job).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Job not found", http.StatusNotFound)
			} else {
				log.Printf("Failed to fetch job from database: %s\n", err)
				http.Error(w, "Failed to fetch job from database", http.StatusInternalServerError)
			}
		}

		// Convert ExecutionTime to requester's local time if timezone is provided
		executedAt := job.ExecutionTime
		if timezone != "" {
			loc, err := time.LoadLocation(timezone)
			if err != nil {
				http.Error(w, "Invalid timezone", http.StatusBadRequest)
			}

			executedAt = executedAt.In(loc)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":          job.ID,
			"command":     job.Command,
			"status":      job.Status,
			"executed_at": executedAt,
		})
	}
}
