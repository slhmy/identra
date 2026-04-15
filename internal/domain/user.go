package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNotFound is returned when a resource is not found.
var ErrNotFound = errors.New("resource not found")

// ErrAlreadyExists is returned when a resource with the same unique key already exists.
var ErrAlreadyExists = errors.New("resource already exists")

// UserModel represents a user entity in the system.
type UserModel struct {
	ID               string         `gorm:"type:varchar(36);primaryKey" bson:"_id,omitempty" json:"id"`
	CreatedAt        time.Time      `bson:"created_at,omitempty" json:"created_at"`
	UpdatedAt        time.Time      `bson:"updated_at,omitempty" json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" bson:"-" json:"-"`
	Email            string         `gorm:"type:varchar(255);uniqueIndex" bson:"email" json:"email"`
	HashedPassword   *string        `gorm:"column:hashed_password" bson:"hashed_password,omitempty" json:"-"`
	VerificationHash *string        `gorm:"column:hash"            bson:"hash,omitempty" json:"-"`
	LastLoginAt      *time.Time     `                              bson:"last_login_at,omitempty" json:"last_login_at"`
}

// TableName returns the database table name for UserModel.
func (UserModel) TableName() string {
	return "users"
}

// BeforeCreate generates a UUID for the user before creating.
func (u *UserModel) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	return nil
}

// UserStore defines the interface for user persistence operations.
type UserStore interface {
	Create(ctx context.Context, user *UserModel) error
	GetByID(ctx context.Context, id string) (*UserModel, error)
	GetByEmail(ctx context.Context, email string) (*UserModel, error)
	Update(ctx context.Context, user *UserModel) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, offset, limit int) ([]*UserModel, error)
	Count(ctx context.Context) (int64, error)
}
