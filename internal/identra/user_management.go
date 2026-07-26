package identra

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	ScopeUsersRead   = "identra.users.read"
	ScopeUsersManage = "identra.users.manage"
)

func (s *Service) ListUsers(ctx context.Context, req *identra_v1_pb.ListUsersRequest) (*identra_v1_pb.ListUsersResponse, error) {
	if _, err := s.authorizeServiceAccount(ctx, ScopeUsersRead, ScopeUsersManage); err != nil {
		return nil, err
	}
	pageSize := int(req.GetPageSize())
	if pageSize == 0 {
		pageSize = 50
	}
	if pageSize < 0 || pageSize > 200 {
		return nil, status.Error(codes.InvalidArgument, "page size must be between 1 and 200")
	}
	offset, err := pageOffset(req.GetPageToken())
	if err != nil {
		return nil, err
	}
	users, err := s.userStore.List(ctx, offset, pageSize+1)
	if err != nil {
		slog.ErrorContext(ctx, "failed to list users", "error", err)
		return nil, status.Error(codes.Internal, "failed to list users")
	}
	response := &identra_v1_pb.ListUsersResponse{}
	if len(users) > pageSize {
		users = users[:pageSize]
		response.NextPageToken = strconv.Itoa(offset + pageSize)
	}
	for _, user := range users {
		response.Users = append(response.Users, managedUserProto(user))
	}
	return response, nil
}

func (s *Service) CreatePasswordUser(ctx context.Context, req *identra_v1_pb.CreatePasswordUserRequest) (*identra_v1_pb.CreatePasswordUserResponse, error) {
	actor, err := s.authorizeServiceAccount(ctx, ScopeUsersManage)
	if err != nil {
		return nil, err
	}
	email := strings.TrimSpace(req.GetEmail())
	if email == "" || req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}
	hash, err := security.HashPassword(req.GetPassword())
	if err != nil {
		slog.ErrorContext(ctx, "failed to hash managed-user password", "error", err)
		return nil, status.Error(codes.Internal, "failed to process password")
	}
	user := &UserModel{Email: email, HashedPassword: &hash}
	if err := s.userStore.Create(ctx, user); err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			return nil, status.Error(codes.AlreadyExists, "user already exists")
		}
		slog.ErrorContext(ctx, "failed to create managed user", "error", err, "email", email)
		return nil, status.Error(codes.Internal, "failed to create user")
	}
	s.recordManagementAudit(ctx, actor, "user.create", "user", user.ID)
	return &identra_v1_pb.CreatePasswordUserResponse{User: managedUserProto(user)}, nil
}

func (s *Service) DeleteUser(ctx context.Context, req *identra_v1_pb.DeleteUserRequest) (*identra_v1_pb.DeleteUserResponse, error) {
	actor, err := s.authorizeServiceAccount(ctx, ScopeUsersManage)
	if err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(req.GetUserId())
	if userID == "" {
		return nil, status.Error(codes.InvalidArgument, "user ID is required")
	}
	if err := s.userStore.Delete(ctx, userID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		slog.ErrorContext(ctx, "failed to delete managed user", "error", err, "user_id", userID)
		return nil, status.Error(codes.Internal, "failed to delete user")
	}
	s.recordManagementAudit(ctx, actor, "user.delete", "user", userID)
	return &identra_v1_pb.DeleteUserResponse{}, nil
}

func (s *Service) ResetUserPassword(ctx context.Context, req *identra_v1_pb.ResetUserPasswordRequest) (*identra_v1_pb.ResetUserPasswordResponse, error) {
	actor, err := s.authorizeServiceAccount(ctx, ScopeUsersManage)
	if err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(req.GetUserId())
	if userID == "" || req.GetPassword() == "" {
		return nil, status.Error(codes.InvalidArgument, "user ID and password are required")
	}
	hash, err := security.HashPassword(req.GetPassword())
	if err != nil {
		slog.ErrorContext(ctx, "failed to hash reset password", "error", err)
		return nil, status.Error(codes.Internal, "failed to process password")
	}
	if err := s.userStore.SetPassword(ctx, userID, hash); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		slog.ErrorContext(ctx, "failed to reset managed-user password", "error", err, "user_id", userID)
		return nil, status.Error(codes.Internal, "failed to reset password")
	}
	user, err := s.userStore.GetByID(ctx, userID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to read user after password reset", "error", err, "user_id", userID)
		return nil, status.Error(codes.Internal, "failed to read user")
	}
	s.recordManagementAudit(ctx, actor, "user.reset_password", "user", userID)
	return &identra_v1_pb.ResetUserPasswordResponse{User: managedUserProto(user)}, nil
}

func pageOffset(token string) (int, error) {
	if strings.TrimSpace(token) == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil || offset < 0 {
		return 0, status.Error(codes.InvalidArgument, "invalid page token")
	}
	return offset, nil
}

func managedUserProto(user *UserModel) *identra_v1_pb.ManagedUser {
	result := &identra_v1_pb.ManagedUser{
		Id:                   user.ID,
		Email:                user.Email,
		PasswordLoginEnabled: user.HashedPassword != nil,
		CreatedAt:            timestamppb.New(user.CreatedAt),
		UpdatedAt:            timestamppb.New(user.UpdatedAt),
	}
	if user.LastLoginAt != nil {
		result.LastLoginAt = timestamppb.New(*user.LastLoginAt)
	}
	return result
}
