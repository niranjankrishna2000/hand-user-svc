package models

import (
	"time"

	"gorm.io/gorm"
)

type Post struct {
	gorm.Model `json:"-"`
	Id         int       `json:"id" gorm:"unique;not null"`
	UserID     int       `json:"userID"`
	Text       string    `json:"text"`
	Place      string    `json:"string"`
	Amount     int       `json:"amount"`
	Collected  int       `json:"collected"`
	Image      string    `json:"image"`
	Date       time.Time `json:"time"`
	Address    string    `json:"address"`
	Account_No string    `json:"accno"`
	CategoryId string    `json:"categoryId"`
	TaxBenefit bool      `json:"tax_benefit" gorm:"default:false"`
}

type Reported struct {
	Id        int    `json:"id" gorm:"unique;not null"`
	UserID    int    `json:"userID"`
	Reason    string `json:"reason"`
	PostID    int    `json:"postID" `
	CommentID int    `json:"commentID"`
	Category  string `json:"category"`
}

type Comment struct {
	Id      int       `json:"id" gorm:"unique;not null"`
	UserID  int       `json:"userID"`
	PostID  int       `json:"postID"`
	Time    time.Time `json:"time"`
	Comment string    `json:"comment" gorm:"not null"`
}

type Notification struct {
	Id     int       `json:"id" gorm:"unique;not null"`
	UserID int       `json:"userID"`
	PostID int       `json:"postID"`
	FromID int       `json:"fromID"`
	Time   time.Time `json:"time"`
	Text   string    `json:"text" gorm:"not null"`
	Type   string    `json:"type"`
}

type Story struct {
	Id      int32     `json:"id" gorm:"unique;notnull"`
	Title   string    `json:"title"`
	Text    string    `json:"text"`
	Place   string    `json:"place"`
	Image   string    `json:"image"`
	Date    time.Time `json:"date"`
	User_id int32     `json:"userid"`
}

type Update struct {
	Id     int32     `json:"id" gorm:"unique;notnull"`
	Title  string    `json:"title"`
	Text   string    `json:"text"`
	Date   time.Time `json:"date"`
	Postid int32     `json:"postid"`
}
