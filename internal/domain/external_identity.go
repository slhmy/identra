package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ExternalIdentityModel represents an OAuth provider identity linked to a user.
// Each row represents the binding (provider, provider_user_id) -> user_id.
type ExternalIdentityModel struct {
	ID             string         `gorm:"type:varchar(36);primaryKey" bson:"_id,omitempty" json:"id"`
	UserID         string         `gorm:"type:varchar(36);index" bson:"user_id" json:"user_id"`
	Provider       string         `gorm:"type:varchar(50)" bson:"provider" json:"provider"`
	ProviderUserID string         `gorm:"type:varchar(255)" bson:"provider_user_id" json:"provider_user_id"`
	CreatedAt      time.Time      `bson:"created_at,omitempty" json:"created_at"`
	UpdatedAt      time.Time      `bson:"updated_at,omitempty" json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" bson:"-" json:"-"`
}

// TableName returns the database table name for ExternalIdentityModel.
func (ExternalIdentityModel) TableName() string {
	return "external_identities"
}

// BeforeCreate generates a UUID for the external identity before creating.
func (e *ExternalIdentityModel) BeforeCreate(_ *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

// ExternalIdentityStore defines the interface for external identity persistence operations.
type ExternalIdentityStore interface {
	// Create persists a new external identity. Returns ErrAlreadyExists if the
	// (provider, provider_user_id) pair is already linked to a user.
	Create(ctx context.Context, identity *ExternalIdentityModel) error
	// GetByProviderID looks up a single external identity by (provider, provider_user_id).
	// Returns ErrNotFound if no match exists.
	GetByProviderID(ctx context.Context, provider, providerUserID string) (*ExternalIdentityModel, error)
	// GetByUserID returns all external identities linked to the given user.
	GetByUserID(ctx context.Context, userID string) ([]*ExternalIdentityModel, error)
}
