package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/arnavsurve/promise/pkg/db"
	"github.com/arnavsurve/promise/pkg/models"
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

func GetJobStatus(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		id := queryParams.Get("id")
		fmt.Printf("%s", id)
	}
}
