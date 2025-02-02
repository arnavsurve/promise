package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/arnavsurve/promise/pkg/ai"
	"github.com/arnavsurve/promise/pkg/db"
	"github.com/arnavsurve/promise/pkg/models"
)

var (
	ctx = context.Background()

	activeWorkers  int32 = 0
	uniqueWorkerId int32 = 0

	maxWorkers int32 = 50
	minWorkers int32 = 1
)

// Overriding min for int32 values for use in the atomic counter
func min(a, b int32) int32 {
	if a < b {
		return a
	}
	return b
}

// checkDependencies checks if all dependencies for a task have been completed.
// It returns a map where keys are in the format 'taskid:subtaskid' and values are the dependency result (context for the next worker).
// If any dependency is not yet complete, ok is false.
func checkDependencies(s *db.Store, task models.Task) (depsContext map[string]string, ok bool) {
	depsContext = make(map[string]string)
	for _, dep := range task.Dependencies {
		// Construct a key for the dependency result
		key := fmt.Sprintf("task_result:%s:%d", task.TaskId, dep.SubtaskId)
		result, err := s.Rdb.Get(ctx, key).Result()
		if err != nil {
			// Dependency result not yet available
			return nil, false
		}

		// Save dependency result in the map
		depKey := fmt.Sprintf("%s:%d", dep.TaskId, dep.SubtaskId)
		depsContext[depKey] = result
	}

	return depsContext, true
}

// storeTaskResult stores the result (context) of a completed task in Redis to be used by the next worker.
func storeTaskResult(s *db.Store, task models.Task, result string) error {
	key := fmt.Sprintf("task_result:%s:%d", task.TaskId, task.SubtaskId)
	// TODO set expiration
	return s.Rdb.Set(ctx, key, result, 0).Err()
}

// startWorker spawns a worker to process tasks. It checks dependencies and passes dependency context to the processing function.
func startWorker(s *db.Store, workerId int32) {
	log.Printf("Worker %d started", workerId)
	idleTimeout := time.NewTimer(10 * time.Second) // Worker shuts down after idling for 10 seconds

	for {
		taskKeys, err := s.Rdb.Keys(ctx, "task_queue:*").Result()
		if err != nil || len(taskKeys) == 0 {
			// Wait for tasks, then check again
			select {
			case <-idleTimeout.C:
				log.Printf("Worker %d terminated due to inactivity", workerId)
				atomic.AddInt32(&activeWorkers, -1)
				return // Exit if no task arrives within timeout
			default:
				time.Sleep(2 * time.Second)
			}
			continue
		}

		for _, taskQueue := range taskKeys {
			result, err := s.Rdb.LPop(ctx, taskQueue).Result()
			if err != nil {
				continue
			}

			var task models.Task
			if err := json.Unmarshal([]byte(result), &task); err != nil {
				log.Printf("Worker %d failed to parse task: %s\n", workerId, err)
				continue
			}

			// Check if dependencies are complete
			depsContext, ready := checkDependencies(s, task)
			if !ready {
				// Not all dependencies have complete, requeue the task
				// log.Printf("Worker %d: Dependencies not complete for subtask %d in task %s - requeueing", workerId, task.SubtaskId, task.TaskId)
				s.Rdb.RPush(ctx, taskQueue, result)
				continue
			}

			// Reset timemout when work is found
			if !idleTimeout.Stop() {
				<-idleTimeout.C
			}
			idleTimeout.Reset(10 * time.Second)

			// Process the task, passing along dependency context
			resultContext, err := ai.ProcessTask(task, depsContext)
			if err != nil {
				log.Printf("Worker %d: Error processing subtask %d in task %s: %v", workerId, task.SubtaskId, task.TaskId, err)
				continue
			}

			// Store task result (context) for dependent tasks to access
			if err := storeTaskResult(s, task, resultContext); err != nil {
				log.Printf("Worker %d: Failed to store result for subtask %d in task %s: %v", workerId, task.SubtaskId, task.TaskId, err)
			}

			log.Printf("Worker %d completed subtask %d in task %s", workerId, task.SubtaskId, task.TaskId)
		}
	}
}

// WorkerManager dynamically adjusts the number of workers
func WorkerManager(s *db.Store) {
	fmt.Println("Starting Worker Manager...")

	for {
		taskKeys, _ := s.Rdb.Keys(ctx, "task_queue:*").Result()
		totalTasks := int32(0)

		for _, key := range taskKeys {
			length, _ := s.Rdb.LLen(ctx, key).Result()
			totalTasks += int32(length)
		}

		currentWorkers := atomic.LoadInt32(&activeWorkers)

		// If there are more tasks than active workers and we have not hit maxWorkers,
		// spawn new workers
		if totalTasks > currentWorkers && currentWorkers < maxWorkers {
			// Spawn new workers
			newWorkers := min(totalTasks-currentWorkers, maxWorkers-currentWorkers)
			for i := int32(0); i < newWorkers; i++ {
				// Generate a unique worker ID
				id := atomic.AddInt32(&uniqueWorkerId, 1)
				atomic.AddInt32(&activeWorkers, 1)
				go startWorker(s, id)
			}
			log.Printf("Scaled up: Spawned %d new workers (Total: %d)\n", newWorkers, atomic.LoadInt32(&activeWorkers))
		} else if totalTasks == 0 && currentWorkers > minWorkers {
			// When no tasks are in queue, idle workers will eventually shut down on their own
			log.Println("Scaled down: Waiting for idle workers to terminate.")
		}

		time.Sleep(5 * time.Second) // Check queue size every 5 seconds
	}
}
