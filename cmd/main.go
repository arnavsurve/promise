package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"

	"github.com/arnavsurve/promise/pkg/db"
	"github.com/arnavsurve/promise/pkg/handlers"
	"github.com/arnavsurve/promise/pkg/workers"
	"github.com/joho/godotenv"
)

func main() {
	numWorkers := flag.Int("w", 0, "Size of worker pool to initialize")
	retryCount := flag.Int("retry", 3, "Retry limit for jobs on failure")

	flag.Parse()

	if err := godotenv.Load(); err != nil {
		log.Printf("error %s", err)
	}

	store, err := db.NewStore()
	if err != nil {
		log.Fatal(err)
	}
	store.InitJobsTable()

	go workers.InitWorkerPool(store, *numWorkers, *retryCount)

	http.HandleFunc("/job", requestHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.EnqueueJob(store),
	}))

	http.HandleFunc("/job/status", handlers.GetJobStatus(store))

	fmt.Print("Server running on :8080\n\n")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// requestHandler handles incoming requests and calls the handler associated with a particular HTTP request method
func requestHandler(handlers map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if handler, exists := handlers[r.Method]; exists {
			handler(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
