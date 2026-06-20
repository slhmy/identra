package domain

import (
	"errors"
	"time"
)

// ErrNotFound is returned when a resource is not found.
var ErrNotFound = errors.New("resource not found")

// ErrAlreadyExists is returned when a resource with the same unique key already exists.
var ErrAlreadyExists = errors.New("resource already exists")

// UserModel represents a user entity in the system.
type UserModel struct {
	ID               string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Email            string
	HashedPassword   *string
	VerificationHash *string
	LastLoginAt      *time.Time
}
