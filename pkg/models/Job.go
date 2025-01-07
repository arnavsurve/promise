package models

import (
	"time"

	"gorm.io/gorm"
)

type Job struct {
	gorm.Model
	Command       string    `json:"command"`
	Status        string    `json:"status"`
	ExecutionTime time.Time `json:"execution_time"`
	RetryCount    int       `json:"retry_count"`
}
