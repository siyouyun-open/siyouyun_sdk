// Package paging is a minimal pagination model.
//
// The field names are part of the frontend contract (page / pageSize) and must not be changed.
// When parsing from an HTTP request, use the iris helper functions in contrib/restjson, so this
// package stays dependency-free and can also be reused in non-Web scenarios (background tasks, migrations).
package paging

// Request parameter names (the frontend and webui pass parameters by these two names).
const (
	PageCurrent = "page"
	PageSize    = "pageSize"
)

// Pagination is the position of one paginated request.
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// New builds pagination parameters and normalizes them: the page number is at least 1, and a non-positive page size falls back to 10.
func New(page, pageSize int) *Pagination {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	return &Pagination{Page: page, PageSize: pageSize}
}

// Offset returns the SQL OFFSET.
func (p *Pagination) Offset() int {
	if p == nil {
		return 0
	}
	page := p.Page
	if page <= 0 {
		page = 1
	}
	size := p.PageSize
	if size <= 0 {
		size = 10
	}
	return (page - 1) * size
}

// Limit returns the SQL LIMIT.
func (p *Pagination) Limit() int {
	if p == nil || p.PageSize <= 0 {
		return 10
	}
	return p.PageSize
}
