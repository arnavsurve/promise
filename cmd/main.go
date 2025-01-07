package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/arnavsurve/promise/pkg/db"
	"github.com/arnavsurve/promise/pkg/handlers"
	"github.com/arnavsurve/promise/pkg/workers"
	"github.com/joho/godotenv"
	// "gorm.io/gorm/logger"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("error %s", err)
	}

	store, err := db.NewStore()
	if err != nil {
		log.Fatal(err)
	}
	store.InitJobsTable()
	// store.DB.Logger = logger.Default.LogMode(logger.Info)

	go workers.ProcessJobs(store)

	http.HandleFunc("/job", requestHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.EnqueueJob(store),
	}))

	http.HandleFunc("/job/status", handlers.GetJobStatus(store))

	fmt.Println("Server running on :8080")
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
