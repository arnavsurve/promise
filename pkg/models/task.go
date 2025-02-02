package models

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Dependency struct {
	TaskId    uuid.UUID `json:"task_id"`
	SubtaskId int       `json:"subtask_id"`
}

type Task struct {
	TaskId       uuid.UUID    `gorm:"type:uuid;not null" json:"task_id"` // Shared by all subtasks
	SubtaskId    int          `gorm:"not null" json:"subtask_id"`        // Unique within TaskId
	Type         string       `gorm:"type:varchar(20);not null" json:"type"`
	Description  string       `json:"description"`
	Dependencies []Dependency `gorm:"serializer:json" json:"dependencies"`
	Status       string       `json:"status"`

	gorm.Model
}

type TaskResponse struct {
	TaskId       uuid.UUID    `json:"task_id"`
	SubtaskId    int          `json:"subtask_id"`
	Type         string       `json:"type"`
	Description  string       `json:"description"`
	Dependencies []Dependency `gorm:"serializer:json" json:"dependencies"`
	Status       string       `json:"status"`
}

type Subtask struct {
	SubtaskId    int    `json:"subtask_id"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	Dependencies []int  `json:"dependencies"`
}
