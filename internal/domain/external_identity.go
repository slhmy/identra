package domain

import "time"

// ExternalIdentityModel represents an OAuth provider identity linked to a user.
// Each row represents the binding (provider, provider_user_id) -> user_id.
type ExternalIdentityModel struct {
	ID             string
	UserID         string
	Provider       string
	ProviderUserID string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
