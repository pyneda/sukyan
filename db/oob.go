package db

import (
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"time"
)

// type OOBSession struct {
// 	gorm.Model
// }

type OOBTest struct {
	gorm.Model
	TestName          string `json:"test_name"`
	Target            string `json:"target"`
	HistoryID         int
	HistoryItem       History
	InteractionDomain string    `json:"interaction_domain"`
	InteractionFullID string    `json:"interaction_id"`
	Payload           string    `json:"payload"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// CreateOOBTest saves an OOBTest to the database
func (d *DatabaseConnection) CreateOOBTest(item OOBTest) (OOBTest, error) {
	result := d.db.Create(&item)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("item", item).Msg("Failed to create OOBTest")
	}
	return item, result.Error
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

// CreateInteraction saves an issue to the database
func (d *DatabaseConnection) CreateInteraction(item OOBInteraction) (OOBInteraction, error) {
	result := d.db.Create(&item)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("interaction", item).Msg("Failed to create interaction")
	}
	return item, result.Error
}
