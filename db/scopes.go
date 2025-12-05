package db

import "gorm.io/gorm"

// Paginate Gorm scope to paginate queries based on Paginator
// If PageSize is 0, pagination is skipped entirely (returns all records)
func Paginate(p *Pagination) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		// Skip pagination if PageSize is 0
		if p.PageSize == 0 {
			return db
		}
		offset, pageSize := p.GetData()
		return db.Offset(offset).Limit(pageSize)
	}
}
