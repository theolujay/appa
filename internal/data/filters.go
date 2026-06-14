package data

import (
	"slices"
	"strings"

	vd "github.com/theolujay/appa/internal/validator"
)

type Filters struct {
	Page         int
	PageSize     int
	Sort         string
	SortSafelist []string
}

type Metadata struct {
	CurrentPage  int `json:"current_page,omitzero"`
	PageSize     int `json:"page_size,omitzero"`
	FirstPage    int `json:"first_page,omitzero"`
	LastPage     int `json:"last_page,omitzero"`
	TotalRecords int `json:"total_records,omitzero"`
}

func ValidateFilters(f Filters) vd.Error {
	v := vd.New()
	v.Check(f.Page > 0, "page", "must be greater than zero")
	v.Check(f.Page <= 1_000_000, "page", "must be a maximum of 10 million")
	v.Check(f.PageSize > 0, "page_size", "must be greater than zero")
	v.Check(f.PageSize <= 100, "page_size", "must be a maximum of 100")
	v.Check(vd.PermittedValue(f.Sort, f.SortSafelist...), "sort", "invalid sort value")
	return v.Errors
}

// The sortColumn() helper checks that the client-provided Sort field
// matches one of the entries in the safelist and, if it does, extracts
// the column name from the Sort field by stripping the leading hyphen
// character (if one exists).
func (f Filters) sortColumn() string {
	if slices.Contains(f.SortSafelist, f.Sort) {
		return strings.TrimPrefix(f.Sort, "-")
	}

	// In theory, this should never be reached since validation would
	// have happened before at call site, but it's a safe fallback to
	// help stop a SQL injection attack from occurring.
	panic("unsafe sort parameter: " + f.Sort)
}

// The sortDirection() helper returns the appropriate direction to sort
// -- ascending or descending -- depending on if the client-provided
// Sort contains a "-" character prefix.
func (f Filters) sortDirection() string {
	if strings.HasPrefix(f.Sort, "-") {
		return "DESC"
	}
	return "ASC"
}

// The limit() helper returns the page_size to be used in
// `LIMIT page_size` in a database query. The LIMIT clause
// allows to set the max number of records that a SQL query
// should return.
func (f Filters) limit() int {
	return f.PageSize
}

// The offset() helper returns `(page - 1) * page_size` to
// be used with `OFFSET` in a database query. The OFFSET
// clause allows to 'skip' a specific number of rows before
// starting to return records from the query.
//
// NOTE: There's the theoretical risk of an integer overflow
// as these two int values are multiplied together.
func (f Filters) offset() int {
	return (f.Page - 1) * f.PageSize
}

// The calculateMetadata() function calculates the appropriate pagination
// metadata values given the total number of records, current page, and
// page size values. Note that when the last page value is calculated two
// int values are divided, and when dividing integer types in Go the result
// will also be an integer type, with the modulus (or remainder) dropped.
// For example,
func calculateMetadata(totalRecords, page, pageSize int) Metadata {
	if totalRecords == 0 {
		return Metadata{}
	}
	return Metadata{
		CurrentPage: page,
		PageSize:    pageSize,
		FirstPage:   1,
		// Because Go rounds integer down, and we need to account for the
		// remaining records in the last page, we add `pageSize - 1` to
		// the numerator, totalRecords. Why not just pageSize, though?
		// Well, think about it, if we had totalRecords = 10 and
		// pageSize = 5, adding just pageSize would give us a last page
		// of three -- (10 + 5) / 5 -- but that would be wrong, since
		// in reality we'll have item 10 on page 2. That's where the
		// `pageSize - 1` trick comes in. It's just enough to have
		// the last page round up if there's at least one item there
		// but still less enough to not have a "phantom" page --
		// (10 + 5 - 1) / 5 = 2.8 (~2)
		LastPage:     (totalRecords + pageSize - 1) / pageSize,
		TotalRecords: totalRecords,
	}
}
