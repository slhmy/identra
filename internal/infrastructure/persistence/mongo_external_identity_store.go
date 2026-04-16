package persistence

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/poly-workshop/identra/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type mongoExternalIdentityStore struct {
	coll *mongo.Collection
}

// NewMongoExternalIdentityStore builds a MongoDB-backed external identity store and ensures indexes.
func NewMongoExternalIdentityStore(
	ctx context.Context,
	client *mongo.Client,
	databaseName string,
	collectionName string,
) (domain.ExternalIdentityStore, error) {
	if client == nil {
		return nil, errors.New("mongo client is required")
	}
	if strings.TrimSpace(databaseName) == "" {
		return nil, errors.New("mongo database name is required")
	}
	if strings.TrimSpace(collectionName) == "" {
		collectionName = "external_identities"
	}

	store := &mongoExternalIdentityStore{
		coll: client.Database(databaseName).Collection(collectionName),
	}
	if err := store.ensureIndexes(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func (r *mongoExternalIdentityStore) ensureIndexes(ctx context.Context) error {
	models := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "provider", Value: 1},
				{Key: "provider_user_id", Value: 1},
			},
			Options: options.Index().SetUnique(true).SetName("idx_provider_user_id_unique"),
		},
		{
			Keys: bson.D{
				{Key: "provider", Value: 1},
				{Key: "user_id", Value: 1},
			},
			Options: options.Index().SetUnique(true).SetName("idx_provider_user_id_per_user_unique"),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetName("idx_user_id"),
		},
	}

	if _, err := r.coll.Indexes().CreateMany(ctx, models); err != nil {
		slog.WarnContext(ctx, "failed to ensure external identity indexes", "error", err)
		return err
	}
	return nil
}

func (r *mongoExternalIdentityStore) Create(
	ctx context.Context,
	identity *domain.ExternalIdentityModel,
) error {
	now := time.Now().UTC()
	if strings.TrimSpace(identity.ID) == "" {
		identity.ID = uuid.New().String()
	}
	if identity.CreatedAt.IsZero() {
		identity.CreatedAt = now
	}
	identity.UpdatedAt = now

	if _, err := r.coll.InsertOne(ctx, identity); err != nil {
		slog.ErrorContext(
			ctx,
			"failed to create external identity (mongo)",
			"error", err,
			"provider", identity.Provider,
			"provider_user_id", identity.ProviderUserID,
		)
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrAlreadyExists
		}
		return err
	}
	slog.InfoContext(
		ctx,
		"external identity created successfully (mongo)",
		"id", identity.ID,
		"user_id", identity.UserID,
		"provider", identity.Provider,
	)
	return nil
}

func (r *mongoExternalIdentityStore) GetByProviderID(
	ctx context.Context,
	provider, providerUserID string,
) (*domain.ExternalIdentityModel, error) {
	var identity domain.ExternalIdentityModel
	err := r.coll.FindOne(ctx, bson.M{
		"provider":         provider,
		"provider_user_id": providerUserID,
	}).Decode(&identity)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		slog.ErrorContext(
			ctx,
			"failed to fetch external identity (mongo)",
			"error", err,
			"provider", provider,
			"provider_user_id", providerUserID,
		)
		return nil, err
	}
	return &identity, nil
}

func (r *mongoExternalIdentityStore) GetByUserID(
	ctx context.Context,
	userID string,
) ([]*domain.ExternalIdentityModel, error) {
	cursor, err := r.coll.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		slog.ErrorContext(ctx, "failed to list external identities (mongo)", "error", err, "user_id", userID)
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var identities []*domain.ExternalIdentityModel
	for cursor.Next(ctx) {
		var identity domain.ExternalIdentityModel
		if decodeErr := cursor.Decode(&identity); decodeErr != nil {
			_ = cursor.Close(ctx)
			return nil, decodeErr
		}
		identities = append(identities, &identity)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return identities, nil
}
