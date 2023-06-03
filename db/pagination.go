package db

const (
	maxPageSize     = 1000
	defaultPageSize = 25
)

// Pagination used to store pagination config
type Pagination struct {
	Page     int
	PageSize int
}

func (p *Pagination) GetData() (offset int, limit int) {
	if p.Page == 0 {
		p.Page = 1
	}
	switch {
	case p.PageSize > maxPageSize:
		p.PageSize = maxPageSize
	case p.PageSize <= 0:
		p.PageSize = defaultPageSize
	}

	offset = (p.Page - 1) * p.PageSize

	return offset, p.PageSize
}
