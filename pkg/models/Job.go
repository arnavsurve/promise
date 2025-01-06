package models

import (
	"gorm.io/gorm"
)

type Job struct {
	gorm.Model
	Command string `json:"command"`
	Status  string `json:"status"`
}
