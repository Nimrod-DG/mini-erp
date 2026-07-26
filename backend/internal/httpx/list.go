package httpx

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// Page size bounds from §9.0. The maximum exists so one client cannot ask for a
// tenant's entire history in a single response.
const (
	DefaultPageSize = 25
	MaxPageSize     = 100
)

// ListParams is a parsed §9.0 list request.
//
// SortColumn is a SQL identifier that came out of the caller's allowlist, never
// out of the request: `sort` names an API field, and ParseList translates. A
// request cannot introduce a column name, so interpolating this into ORDER BY
// is not an injection — accepting the raw parameter would be.
type ListParams struct {
	Page       int
	PageSize   int
	Q          string
	SortColumn string
	SortDesc   bool
}

// Offset is the SQL OFFSET for this page.
func (p ListParams) Offset() int { return (p.Page - 1) * p.PageSize }

// OrderBy renders the ORDER BY clause, tie-broken by the sort column plus a
// stable second key the caller supplies. Without a tie-break, two rows with
// equal sort keys can swap between page 1 and page 2 and a row is silently
// never shown.
func (p ListParams) OrderBy(tieBreak string) string {
	dir := "ASC"
	if p.SortDesc {
		dir = "DESC"
	}
	return fmt.Sprintf("%s %s, %s ASC", p.SortColumn, dir, tieBreak)
}

// Like is the value to bind against an ILIKE placeholder, or "%" when the
// caller sent no search term.
func (p ListParams) Like() string {
	if p.Q == "" {
		return "%"
	}
	return "%" + p.Q + "%"
}

// ListResponse is the §9.0 envelope. Every list endpoint returns this shape so
// the frontend can share one hook and one pagination component.
//
// TotalItems is mandatory, not optional: §10.7.4 requires a visible total, and
// "Page 3 of ?" strands people.
type ListResponse[T any] struct {
	Data       []T   `json:"data"`
	Page       int   `json:"page"`
	PageSize   int   `json:"pageSize"`
	TotalItems int64 `json:"totalItems"`
	TotalPages int   `json:"totalPages"`
}

// NewListResponse wraps one page of rows.
func NewListResponse[T any](data []T, p ListParams, totalItems int64) ListResponse[T] {
	if data == nil {
		// `[]` rather than `null`: the frontend maps over this.
		data = []T{}
	}
	totalPages := int((totalItems + int64(p.PageSize) - 1) / int64(p.PageSize))
	return ListResponse[T]{
		Data:       data,
		Page:       p.Page,
		PageSize:   p.PageSize,
		TotalItems: totalItems,
		TotalPages: totalPages,
	}
}

// ParseList reads `?page=&pageSize=&sort=&q=` (§9.0).
//
// `sortable` maps API field names to SQL columns and is the allowlist;
// `defaultSort` must be one of its keys, optionally `-` prefixed for
// descending. An unknown sort field is an error rather than a silent fallback:
// sorting by something other than what was asked for misrepresents the data,
// which §9.0 rates as worse than not offering sort at all.
func ParseList(c *fiber.Ctx, sortable map[string]string, defaultSort string) (ListParams, error) {
	p := ListParams{
		Page:     c.QueryInt("page", 1),
		PageSize: c.QueryInt("pageSize", DefaultPageSize),
		Q:        strings.TrimSpace(c.Query("q")),
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.PageSize < 1 {
		p.PageSize = DefaultPageSize
	}
	if p.PageSize > MaxPageSize {
		p.PageSize = MaxPageSize
	}

	sort := c.Query("sort", defaultSort)
	field := strings.TrimPrefix(sort, "-")
	p.SortDesc = strings.HasPrefix(sort, "-")

	column, ok := sortable[field]
	if !ok {
		return ListParams{}, fmt.Errorf("cannot sort by %q", field)
	}
	p.SortColumn = column
	return p, nil
}
