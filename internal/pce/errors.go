package pce

import (
	"errors"
	"fmt"
	"time"
)

// ErrLabelNotFound is returned when a label lookup finds no exact match.
var ErrLabelNotFound = errors.New("illumio label not found")

// APIError is a non-2xx, non-429 PCE response.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("pce api error: status %d: %s", e.StatusCode, e.Body)
}

// RateLimitError is a 429 from the PCE (limit is 500 req/min per key).
type RateLimitError struct {
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("pce rate limited, retry after %s", e.RetryAfter)
}
