package db

import (
	"time"
	"github.com/rs/zerolog/log"
)

type Task struct {
	BaseModel
	Status string `json:"status"`
	StartedAt time.Time `json:"started_at"`
}


type TaskFilter struct {
	Statuses   []string
	Pagination Pagination
}

func (d *DatabaseConnection) ListTasks(filter TaskFilter) (items []*Task, count int64, err error) {
	filterQuery := make(map[string]interface{})

	if len(filter.Statuses) > 0 {
		filterQuery["status"] = filter.Statuses
	}

	if filterQuery != nil && len(filterQuery) > 0 {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Where(filterQuery).Order("created_at desc").Find(&items).Error
		d.db.Model(&Task{}).Where(filterQuery).Count(&count)

	} else {
		err = d.db.Scopes(Paginate(&filter.Pagination)).Order("created_at desc").Find(&items).Error
		d.db.Model(&Task{}).Count(&count)
	}

	log.Info().Interface("filters", filter).Int("gathered", len(items)).Int("count", int(count)).Int("total_results", len(items)).Msg("Getting task items")

	return items, count, err
}
