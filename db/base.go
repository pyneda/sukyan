package db

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type BaseModel struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

type BaseUUIDModel struct {
	ID        uuid.UUID      `gorm:"type:uuid;default:uuid_generate_v4()" json:"id" validate:"required,uuid"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}
