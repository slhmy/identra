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

type mongoUserStore struct {
	coll *mongo.Collection
}

// NewMongoUserStore builds a MongoDB-backed user repository and ensures indexes.
func NewMongoUserStore(
	ctx context.Context,
	client *mongo.Client,
	databaseName string,
	collectionName string,
) (domain.UserStore, error) {
	if client == nil {
		return nil, errors.New("mongo client is required")
	}
	if strings.TrimSpace(databaseName) == "" {
		return nil, errors.New("mongo database name is required")
	}
	if strings.TrimSpace(collectionName) == "" {
		collectionName = "users"
	}

	repo := &mongoUserStore{
		coll: client.Database(databaseName).Collection(collectionName),
	}
	if err := repo.ensureIndexes(ctx); err != nil {
		return nil, err
	}
	return repo, nil
}

func isIndexNotFoundError(err error) bool {
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) && cmdErr.Code == 27 {
		return true
	}

	return strings.Contains(strings.ToLower(err.Error()), "index not found")
}

func (r *mongoUserStore) ensureIndexes(ctx context.Context) error {
	// Drop the stale github_id index left over from the old schema, if present.
	// Ignore only the expected "index not found" case so fresh deployments and
	// upgraded ones both work, but fail fast for operational problems.
	if err := r.coll.Indexes().DropOne(ctx, "idx_github_id_unique"); err != nil {
		if isIndexNotFoundError(err) {
			slog.DebugContext(ctx, "stale github_id index not present", "error", err)
		} else {
			slog.WarnContext(ctx, "failed to drop stale github_id index", "error", err)
			return err
		}
	}

	models := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "email", Value: 1}},
			// Sparse index allows multiple documents with NULL/empty email values
			// while maintaining uniqueness constraint for non-empty values.
			// This enables OAuth users without email to be created.
			Options: options.Index().SetUnique(true).SetSparse(true).SetName("idx_email_unique"),
		},
	}

	if _, err := r.coll.Indexes().CreateMany(ctx, models); err != nil {
		slog.WarnContext(ctx, "failed to ensure user indexes", "error", err)
		return err
	}
	return nil
}

func (r *mongoUserStore) Create(ctx context.Context, user *domain.UserModel) error {
	r.populateForCreate(user)

	if _, err := r.coll.InsertOne(ctx, user); err != nil {
		slog.ErrorContext(ctx, "failed to create user (mongo)", "error", err, "email", user.Email)
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrAlreadyExists
		}
		return err
	}
	slog.InfoContext(ctx, "user created successfully (mongo)", "user_id", user.ID, "email", user.Email)
	return nil
}

func (r *mongoUserStore) GetByID(ctx context.Context, id string) (*domain.UserModel, error) {
	return r.findOne(ctx, bson.M{"_id": id}, "user_id", id)
}

func (r *mongoUserStore) GetByEmail(ctx context.Context, email string) (*domain.UserModel, error) {
	return r.findOne(ctx, bson.M{"email": email}, "email", email)
}

func (r *mongoUserStore) Update(ctx context.Context, user *domain.UserModel) error {
	if strings.TrimSpace(user.ID) == "" {
		return errors.New("user id is required for update")
	}

	r.populateForUpdate(user)

	result, err := r.coll.ReplaceOne(ctx, bson.M{"_id": user.ID}, user)
	if err != nil {
		slog.ErrorContext(ctx, "failed to update user (mongo)", "error", err, "user_id", user.ID)
		return err
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	slog.DebugContext(ctx, "user updated successfully (mongo)", "user_id", user.ID, "email", user.Email)
	return nil
}

func (r *mongoUserStore) Delete(ctx context.Context, id string) error {
	result, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *mongoUserStore) List(ctx context.Context, offset, limit int) ([]*domain.UserModel, error) {
	opts := options.Find()
	if offset > 0 {
		opts.SetSkip(int64(offset))
	}
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	opts.SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := r.coll.Find(ctx, bson.M{}, opts)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list users (mongo)", "error", err, "offset", offset, "limit", limit)
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var users []*domain.UserModel
	for cursor.Next(ctx) {
		var user domain.UserModel
		if decodeErr := cursor.Decode(&user); decodeErr != nil {
			_ = cursor.Close(ctx)
			return nil, decodeErr
		}
		users = append(users, &user)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	slog.DebugContext(ctx, "users listed successfully (mongo)", "count", len(users), "offset", offset, "limit", limit)
	return users, nil
}

func (r *mongoUserStore) Count(ctx context.Context) (int64, error) {
	count, err := r.coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *mongoUserStore) findOne(ctx context.Context, filter bson.M, key string, value any) (*domain.UserModel, error) {
	var user domain.UserModel
	err := r.coll.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		slog.ErrorContext(ctx, "failed to fetch user (mongo)", "error", err, key, value)
		return nil, err
	}
	slog.DebugContext(ctx, "user retrieved successfully (mongo)", "user_id", user.ID, "email", user.Email)
	return &user, nil
}

func (r *mongoUserStore) populateForCreate(user *domain.UserModel) {
	now := time.Now().UTC()
	if strings.TrimSpace(user.ID) == "" {
		user.ID = uuid.New().String()
	}
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now
}

func (r *mongoUserStore) populateForUpdate(user *domain.UserModel) {
	user.UpdatedAt = time.Now().UTC()
}
