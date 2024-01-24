package models

import (
	"time"

	"gorm.io/gorm"
)

type Payment struct {
	gorm.Model `json:"-"`
	Id         int       `json:"id" gorm:"unique;not null"`
	UserID     int       `json:"userID"`
	PostID     int       `json:"postID"`
	Amount     int       `json:"amount"`
	Date       time.Time `json:"time"`
	Status     string    `json:"status" gorm:"default:'pending'"`
	PaymentID  string    `json:"payment_id"`
}
