package db

import "gorm.io/gorm"

// Paginate Gorm scope to paginate queries based on Paginator
func Paginate(p *Pagination) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		offset, pageSize := p.GetData()
		return db.Offset(offset).Limit(pageSize)
	}
}
