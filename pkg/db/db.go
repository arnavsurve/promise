package db

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/arnavsurve/promise/pkg/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Store struct {
	DB *gorm.DB
}

func NewStore() (*Store, error) {
	host := os.Getenv("DB_HOST")
	port, _ := strconv.Atoi(os.Getenv("DB_PORT"))
	user := os.Getenv("DB_USER")
	dbname := os.Getenv("DB_NAME")
	password := os.Getenv("DB_PASS")

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, password, host, port, dbname)
	db, err := gorm.Open(postgres.Open(connStr), &gorm.Config{
		// Logger: logger.Default.LogMode(logger.Info),
		Logger: logger.Discard,
	})
	if err != nil {
		return nil, err
	}

	fmt.Println("DB connection successful")

	return &Store{
		DB: db,
	}, nil
}

func (s *Store) InitJobsTable() {
	err := s.DB.AutoMigrate(&models.Job{})
	if err != nil {
		log.Fatalf("Error creating accounts table: %v", err)
	}
}
