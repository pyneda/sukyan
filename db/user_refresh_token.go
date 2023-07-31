package db

import (
	"github.com/google/uuid"
)

type RefreshToken struct {
	BaseUUIDModel
	UserID uuid.UUID `gorm:"type:uuid;not null"`
	Token  string    `gorm:"type:text;not null"`
}

func (d *DatabaseConnection) CreateRefreshToken(refreshToken *RefreshToken) error {
	return d.db.Create(refreshToken).Error
}

func (d *DatabaseConnection) DeleteRefreshToken(userID uuid.UUID) error {
	return d.db.Where("user_id = ?", userID).Delete(&RefreshToken{}).Error
}

func (d *DatabaseConnection) GetRefreshToken(userID uuid.UUID) (*RefreshToken, error) {
	var refreshToken RefreshToken
	if err := d.db.Where("user_id = ?", userID).First(&refreshToken).Error; err != nil {
		return nil, err
	}
	return &refreshToken, nil
}

func (d *DatabaseConnection) SaveRefreshToken(userID uuid.UUID, token string) error {
	refreshToken := &RefreshToken{
		UserID: userID,
		Token:  token,
	}
	return d.CreateRefreshToken(refreshToken)
}
