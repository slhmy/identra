package store

import (
	"time"

	"github.com/google/uuid"
	"github.com/slhmy/identra/internal/identra"
	"gorm.io/gorm"
)

type userRecord struct {
	ID               string         `gorm:"type:varchar(36);primaryKey" bson:"_id,omitempty"`
	CreatedAt        time.Time      `bson:"created_at,omitempty"`
	UpdatedAt        time.Time      `bson:"updated_at,omitempty"`
	DeletedAt        gorm.DeletedAt `gorm:"index" bson:"-"`
	Email            string         `gorm:"type:varchar(255);uniqueIndex" bson:"email"`
	HashedPassword   *string        `gorm:"column:hashed_password" bson:"hashed_password,omitempty"`
	VerificationHash *string        `gorm:"column:hash" bson:"hash,omitempty"`
	LastLoginAt      *time.Time     `bson:"last_login_at,omitempty"`
}

func (userRecord) TableName() string {
	return "users"
}

// AutoMigrateGorm migrates the database schema for the GORM-backed stores.
func AutoMigrateGorm(db *gorm.DB) error {
	return db.AutoMigrate(&userRecord{}, &externalIdentityRecord{})
}

func (u *userRecord) BeforeCreate(_ *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

func userRecordFromDomain(user *identra.UserModel) userRecord {
	if user == nil {
		return userRecord{}
	}
	return userRecord{
		ID:               user.ID,
		CreatedAt:        user.CreatedAt,
		UpdatedAt:        user.UpdatedAt,
		Email:            user.Email,
		HashedPassword:   user.HashedPassword,
		VerificationHash: user.VerificationHash,
		LastLoginAt:      user.LastLoginAt,
	}
}

func userRecordToDomain(record userRecord) *identra.UserModel {
	return &identra.UserModel{
		ID:               record.ID,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
		Email:            record.Email,
		HashedPassword:   record.HashedPassword,
		VerificationHash: record.VerificationHash,
		LastLoginAt:      record.LastLoginAt,
	}
}

func copyUserRecordToDomain(record userRecord, user *identra.UserModel) {
	if user == nil {
		return
	}
	*user = *userRecordToDomain(record)
}

type externalIdentityRecord struct {
	ID             string         `gorm:"type:varchar(36);primaryKey" bson:"_id,omitempty"`
	UserID         string         `gorm:"type:varchar(36);index" bson:"user_id"`
	Provider       string         `gorm:"type:varchar(50);uniqueIndex:idx_external_identities_provider_provider_user_id" bson:"provider"`
	ProviderUserID string         `gorm:"type:varchar(255);uniqueIndex:idx_external_identities_provider_provider_user_id" bson:"provider_user_id"`
	CreatedAt      time.Time      `bson:"created_at,omitempty"`
	UpdatedAt      time.Time      `bson:"updated_at,omitempty"`
	DeletedAt      gorm.DeletedAt `gorm:"index" bson:"-"`
}

func (externalIdentityRecord) TableName() string {
	return "external_identities"
}

func (e *externalIdentityRecord) BeforeCreate(_ *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	return nil
}

func externalIdentityRecordFromDomain(identity *identra.ExternalIdentityModel) externalIdentityRecord {
	if identity == nil {
		return externalIdentityRecord{}
	}
	return externalIdentityRecord{
		ID:             identity.ID,
		UserID:         identity.UserID,
		Provider:       identity.Provider,
		ProviderUserID: identity.ProviderUserID,
		CreatedAt:      identity.CreatedAt,
		UpdatedAt:      identity.UpdatedAt,
	}
}

func externalIdentityRecordToDomain(record externalIdentityRecord) *identra.ExternalIdentityModel {
	return &identra.ExternalIdentityModel{
		ID:             record.ID,
		UserID:         record.UserID,
		Provider:       record.Provider,
		ProviderUserID: record.ProviderUserID,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

func copyExternalIdentityRecordToDomain(record externalIdentityRecord, identity *identra.ExternalIdentityModel) {
	if identity == nil {
		return
	}
	*identity = *externalIdentityRecordToDomain(record)
}
