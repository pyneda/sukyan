package db

import (
	"gorm.io/gorm"
	"time"
)

// type OOBSession struct {
// 	gorm.Model
// }

type OOBTest struct {
	gorm.Model
	TestName    string
	HistoryID   int
	HistoryItem History
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type OOBInteraction struct {
	gorm.Model

	OOBTestID int
	OOBTest   OOBTest

	Protocol      string
	FullID        string
	UniqueID      string
	QType         string
	RawRequest    string
	RawResponse   string
	RemoteAddress string
	Timestamp     time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}
