package db

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

type User struct {
	BaseUUIDModel
	Email string `gorm:"type:varchar(255);not null;unique" json:"email" validate:"required,email,lte=255"`
	// Never serialised: this struct is returned by the users list endpoint.
	PasswordHash string `json:"-"`
	Active       bool   `json:"active" validate:"required,len=1"`
	// Superuser grants access to deployment-wide administration. Read from the
	// database on every request rather than carried in the JWT, so a demotion
	// takes effect immediately instead of at token renewal.
	Superuser bool `gorm:"not null;default:false" json:"superuser"`
	// TokensValidFrom revokes every session issued before it. Sessions are
	// otherwise stateless, so this is the only lever that ends them early.
	TokensValidFrom time.Time  `gorm:"not null;default:'epoch'" json:"tokens_valid_from"`
	LastLoginAt     *time.Time `json:"last_login_at"`
}

func (u User) role() string {
	if u.Superuser {
		return "superuser"
	}
	return "member"
}

func (u User) status() string {
	if u.Active {
		return "active"
	}
	return "disabled"
}

func (u User) lastLogin() string {
	if u.LastLoginAt == nil {
		return "never"
	}
	return u.LastLoginAt.Format(time.RFC3339)
}

func (u User) String() string {
	return fmt.Sprintf("%s (%s, %s) last login: %s", u.Email, u.role(), u.status(), u.lastLogin())
}

func (u User) TableHeaders() []string {
	return []string{"Email", "Role", "Status", "Last login", "Joined", "ID"}
}

func (u User) TableRow() []string {
	return []string{
		u.Email,
		u.role(),
		u.status(),
		u.lastLogin(),
		u.CreatedAt.Format(time.RFC3339),
		u.ID.String(),
	}
}

func (u User) Pretty() string {
	role, status, lastLogin := u.role(), u.status(), u.lastLogin()
	return fmt.Sprintf("%sEmail:%s %s, %sRole:%s %s, %sStatus:%s %s, %sLast login:%s %s, %sID:%s %s",
		lib.Blue, lib.ResetColor, u.Email,
		lib.Blue, lib.ResetColor, role,
		lib.Blue, lib.ResetColor, status,
		lib.Blue, lib.ResetColor, lastLogin,
		lib.Blue, lib.ResetColor, u.ID)
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
		log.Debug().Err(err).Str("email", email).Msg("Unable to fetch user by email")
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

func (d *DatabaseConnection) TouchUserLastLogin(id uuid.UUID) error {
	now := time.Now()
	if err := d.db.Model(&User{}).Where("id = ?", id).Update("last_login_at", now).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Unable to record user last login")
		return err
	}
	return nil
}

func (d *DatabaseConnection) DeactivateUser(id uuid.UUID) error {
	if err := d.db.Model(&User{}).Where("id = ?", id).Update("active", false).Error; err != nil {
		log.Error().Err(err).Interface("id", id).Msg("Unable to deactivate user")
		return err
	}
	return nil
}
