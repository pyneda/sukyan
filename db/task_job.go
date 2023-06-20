package db

import (
	"github.com/rs/zerolog/log"
	"time"
)

type TaskJob struct {
	BaseModel
	Title  string `json:"title"`
	TaskID uint   `json:"task_id"`
	Task   Task   `json:"-" gorm:"foreignKey:TaskID"`

	Status      string    `json:"status"`
	CompletedAt time.Time `json:"completed_at"`
}

type TaskJobFilter struct {
	Statuses    []string
	Titles      []string
	CompletedAt *time.Time
	Pagination  Pagination
}

func (d *DatabaseConnection) ListTaskJobs(filter TaskJobFilter) (items []*TaskJob, count int64, err error) {
	filterQuery := make(map[string]interface{})

	if len(filter.Statuses) > 0 {
		filterQuery["status"] = filter.Statuses
	}

	if len(filter.Titles) > 0 {
		filterQuery["title"] = filter.Titles
	}

	if filter.CompletedAt != nil {
		filterQuery["completed_at"] = filter.CompletedAt
	}

	if filterQuery != nil && len(filterQuery) > 0 {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Order("created_at desc").Find(&items).Error
		d.db.Model(&TaskJob{}).Where(filterQuery).Count(&count)
	} else {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Order("created_at desc").Find(&items).Error
		d.db.Model(&TaskJob{}).Count(&count)
	}

	log.Info().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Int("total_results", len(items)).Msg("Getting task job items")

	return items, count, err
}
