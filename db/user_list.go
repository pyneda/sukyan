package db

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	userRecentWindow  = 7 * 24 * time.Hour
	userDormantWindow = 90 * 24 * time.Hour
)

type UserListFilter struct {
	Query      string     `json:"query"`
	SortBy     string     `json:"sort_by"`
	SortOrder  string     `json:"sort_order"`
	Pagination Pagination `json:"pagination"`
}

// UserSortFields are the columns ListUsers can sort by. The API layer validates
// against this so the two cannot drift.
var UserSortFields = []string{"email", "last_login", "created_at", "status", "superuser"}

// userSortColumns maps a sort field onto SQL. last_login is NULLS LAST in both
// directions so never-signed-in accounts never displace real activity.
var userSortColumns = map[string]string{
	"email":      "email",
	"last_login": "last_login_at %s NULLS LAST",
	"created_at": "created_at",
	"status":     "active",
	"superuser":  "superuser",
}

// UserRosterSummary describes the whole deployment. It deliberately ignores
// UserListFilter.Query: it is a roster overview, and recomputing it per search
// would make the header read as if accounts had disappeared.
type UserRosterSummary struct {
	Total          int64 `json:"total"`
	Active         int64 `json:"active"`
	Disabled       int64 `json:"disabled"`
	Superusers     int64 `json:"superusers"`
	SignedInLast7d int64 `json:"signed_in_last_7d"`
	NeverSignedIn  int64 `json:"never_signed_in"`
	Dormant        int64 `json:"dormant"`
}

func (d *DatabaseConnection) ListUsers(filter UserListFilter) ([]User, int64, UserRosterSummary, error) {
	var summary UserRosterSummary

	q := d.db.Model(&User{})
	if filter.Query != "" {
		q = q.Where("email ILIKE ?", "%"+filter.Query+"%")
	}

	var count int64
	if err := q.Count(&count).Error; err != nil {
		log.Error().Err(err).Msg("Unable to count users")
		return nil, 0, summary, err
	}

	order := "email asc"
	if column, ok := userSortColumns[filter.SortBy]; ok {
		direction := "asc"
		if filter.SortOrder == "desc" {
			direction = "desc"
		}
		if filter.SortBy == "last_login" {
			order = fmt.Sprintf(column, direction)
		} else {
			order = column + " " + direction
		}
	}

	offset, limit := filter.Pagination.GetData()

	var users []User
	if err := q.Order(order).Offset(offset).Limit(limit).Find(&users).Error; err != nil {
		log.Error().Err(err).Msg("Unable to list users")
		return nil, 0, summary, err
	}

	summary, err := d.userRosterSummary()
	if err != nil {
		return nil, 0, summary, err
	}
	return users, count, summary, nil
}

func (d *DatabaseConnection) userRosterSummary() (UserRosterSummary, error) {
	var summary UserRosterSummary
	now := time.Now()

	err := d.db.Model(&User{}).Select(
		`COUNT(*) AS total,
		 COUNT(*) FILTER (WHERE active) AS active,
		 COUNT(*) FILTER (WHERE NOT active) AS disabled,
		 COUNT(*) FILTER (WHERE superuser) AS superusers,
		 COUNT(*) FILTER (WHERE active AND last_login_at >= ?) AS signed_in_last7d,
		 COUNT(*) FILTER (WHERE active AND last_login_at IS NULL) AS never_signed_in,
		 COUNT(*) FILTER (WHERE active AND last_login_at < ?) AS dormant`,
		now.Add(-userRecentWindow), now.Add(-userDormantWindow),
	).Scan(&summary).Error
	if err != nil {
		log.Error().Err(err).Msg("Unable to summarise the user roster")
	}
	return summary, err
}

func (d *DatabaseConnection) SetUserSuperuser(email string, superuser bool) (*User, error) {
	result := d.db.Model(&User{}).Where("email = ?", email).Update("superuser", superuser)
	if result.Error != nil {
		log.Error().Err(result.Error).Str("email", email).Msg("Unable to set user superuser flag")
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("no user found with email %q", email)
	}
	return d.GetUserByEmail(email)
}
