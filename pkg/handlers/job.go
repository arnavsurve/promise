package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/arnavsurve/promise/pkg/ai"
	"github.com/arnavsurve/promise/pkg/db"
	"github.com/arnavsurve/promise/pkg/models"
	"github.com/google/uuid"
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
		err = json.NewEncoder(w).Encode(map[string]interface{}{
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

func EnqueueJobWithDecomposition(s *db.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		var job struct {
			Description string `json:"description"`
		}

		err := json.NewDecoder(r.Body).Decode(&job)
		if err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		// Generate a unique task ID
		taskId := uuid.New()

		// Query AI for subtasks
		tasks, err := ai.LLMDecompositionQuery(job.Description)
		if err != nil {
			log.Println(err)
			http.Error(w, "Failed to generate subtasks", http.StatusInternalServerError)
			return
		}

		tx := s.DB.Begin()

		var taskResponses []models.TaskResponse

		// Store subtasks in Postgres
		for _, task := range tasks {

			// Cast TaskResponse to DB model Task
			taskInDb := models.Task{
				TaskId:       taskId,
				SubtaskId:    task.SubtaskId,
				Type:         task.Type,
				Description:  task.Description,
				Dependencies: task.Dependencies,
				Status:       "pending",
			}

			if err := tx.Create(&taskInDb).Error; err != nil {
				log.Printf("Failed to store subtask: %v\n", err)
				tx.Rollback()
				http.Error(w, "Failed to store subtask", http.StatusInternalServerError)
				return
			}

			err = PublishTask(s, taskInDb)
			if err != nil {
				log.Printf("Error publishing task: %s\n", err)
				tx.Rollback()
				http.Error(w, "Failed to publish task to queue", http.StatusInternalServerError)
				return
			}

			taskResponse := models.TaskResponse{
				TaskId:       task.TaskId,
				SubtaskId:    task.SubtaskId,
				Type:         task.Type,
				Description:  task.Description,
				Dependencies: task.Dependencies,
				Status:       task.Status,
			}

			taskResponses = append(taskResponses, taskResponse)
		}

		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "Job decomposed and queued successfully",
			"tasks":   taskResponses,
		})
	}
}

// Publish subtask to Redis
func PublishTask(s *db.Store, task models.Task) error {
	taskJSON, err := json.Marshal(task)
	if err != nil {
		return err
	}

	// Store the task's dependencies
	for _, dep := range task.Dependencies {
		depKey := fmt.Sprintf("task_dependency:%s:%d", task.TaskId, task.SubtaskId)
		s.Rdb.SAdd(ctx, depKey, fmt.Sprintf("%s:%d", dep.TaskId, dep.SubtaskId))
	}

	// Store the subtask status as "pending"
	statusKey := fmt.Sprintf("task_status:%s:%d", task.TaskId, task.SubtaskId)
	s.Rdb.Set(ctx, statusKey, "pending", 0)

	// Push task into the queue for that task ID
	queueKey := fmt.Sprintf("task_queue:%s", task.TaskId)
	err = s.Rdb.RPush(ctx, queueKey, taskJSON).Err()
	if err != nil {
		return err
	}

	return nil
}
