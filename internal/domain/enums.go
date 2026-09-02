package domain

import "fmt"

// Status is the ticket lifecycle state.
type Status string

const (
	StatusOpen   Status = "open"
	StatusWip    Status = "wip"
	StatusDone   Status = "done"
	StatusClosed Status = "closed"
)

// ParseStatus parses one of open|wip|done|closed.
func ParseStatus(s string) (Status, error) {
	switch Status(s) {
	case StatusOpen, StatusWip, StatusDone, StatusClosed:
		return Status(s), nil
	default:
		return "", fmt.Errorf("статус — один из: open wip done closed")
	}
}

// Type is the ticket category.
type Type string

const (
	TypeBUG Type = "BUG"
	TypeOPS Type = "OPS"
	TypeTD  Type = "TD"
	TypeENH Type = "ENH"
)

// ParseType parses one of BUG|OPS|TD|ENH.
func ParseType(s string) (Type, error) {
	switch Type(s) {
	case TypeBUG, TypeOPS, TypeTD, TypeENH:
		return Type(s), nil
	default:
		return "", fmt.Errorf("тип — один из: BUG OPS TD ENH")
	}
}

// Priority is the ticket urgency.
type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

// ParsePriority parses one of low|normal|high.
func ParsePriority(s string) (Priority, error) {
	switch Priority(s) {
	case PriorityLow, PriorityNormal, PriorityHigh:
		return Priority(s), nil
	default:
		return "", fmt.Errorf("приоритет — один из: low normal high")
	}
}
