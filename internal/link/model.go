package link

import (
	"time"

	"github.com/google/uuid"
)

type Link struct {
	ID             uuid.UUID
	Code           string
	DestinationURL string
	ClickCount     int64
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
