package domain

// ID is used across domain entities.
type ID int64

// Status represents a lightweight state value.
type Status string

// Pagination carries paging params and totals.
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total,omitempty"`
}

// Sort defines sorting preference.
type Sort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // asc / desc
}

// Filter expresses a simple filter clause.
type Filter struct {
	Field string `json:"field"`
	Op    string `json:"op"` // eq, like, gt, lt, etc.
	Value any    `json:"value"`
}

// RequestContext carries authenticated user info when available.
type RequestContext struct {
	UserID ID     `json:"userId"`
	Role   string `json:"role"`
}
