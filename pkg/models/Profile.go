package models

import "time"

type MonthlyGoal struct {
	UserID   int    `json:"userID" gorm:"not null,unique"`
	Amount   int    `json:"postID" gorm:"not null"`
	Day      int    `json:"day" gorm:"not null"`
	Category int `json:"comment" gorm:"not null"`
}
type User struct {
	Name           string    `json:"name" gorm:"not null"`
	Email          string    `json:"email" gorm:"not null,unique"`
	Phone          string    `json:"phone" gorm:"not null,unique"`
	Status         string    `json:"status" gorm:"not null"`
	Id             int32     `json:"id"`
	Gender         string    `json:"gender"`
	Dob            time.Time `json:"dob"`
	Address        string    `json:"address"`
	PAN            string    `json:"pan"`
	ProfilePicture string    `json:"profilepic"`
}
