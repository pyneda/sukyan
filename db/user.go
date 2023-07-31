package db

import (
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type User struct {
	BaseUUIDModel
	Email        string `gorm:"type:varchar(255);not null;unique" json:"email" validate:"required,email,lte=255"`
	PasswordHash string `gorm:"<-:false" json:"password_hash,omitempty"`
	Active       bool   `json:"active" validate:"required,len=1"`
}

func (d *DatabaseConnection) CreateUser(user *User) (*User, error) {
	result := d.db.Create(&user)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("user", user).Msg("User creation failed")
	}
	return user, result.Error
}

func (d *DatabaseConnection) GetUserByEmail(email string) (*User, error) {
	var user User
	if err := d.db.Where("email = ?", email).First(&user).Error; err != nil {
		log.Error().Err(err).Str("email", email).Msg("Unable to fetch user by email")
		return nil, err
	}
	return &user, nil
}

func (d *DatabaseConnection) GetUserByID(id uuid.UUID) (*User, error) {
	var user User
	if err := d.db.Where("id = ?", id).First(&user).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Unable to fetch user by ID")
		return nil, err
	}
	return &user, nil
}

func (d *DatabaseConnection) DeactivateUser(id uuid.UUID) error {
	if err := d.db.Model(&User{}).Where("id = ?", id).Update("active", false).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Unable to deactivate user")
		return err
	}
	return nil
}
