package db

const (
	maxPageSize     = 1000
	defaultPageSize = 25
)

// Pagination used to store pagination config
type Pagination struct {
	Page     int `json:"page" validate:"min=1"`
	PageSize int `json:"page_size" validate:"min=1,max=100000"`
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
