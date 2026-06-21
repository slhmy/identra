package identra

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) RegisterByPassword(
	ctx context.Context,
	req *identra_v1_pb.RegisterByPasswordRequest,
) (*identra_v1_pb.RegisterByPasswordResponse, error) {
	email := strings.TrimSpace(req.GetEmail())
	password := req.GetPassword()
	if email == "" || password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	_, err := s.userStore.GetByEmail(ctx, email)
	switch {
	case err == nil:
		return nil, status.Error(codes.AlreadyExists, "user already exists")
	case errors.Is(err, ErrNotFound):
		// expected — proceed to create
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	hash, hashErr := security.HashPassword(password)
	if hashErr != nil {
		slog.ErrorContext(ctx, "failed to hash password", "error", hashErr)
		return nil, status.Error(codes.Internal, "failed to process password")
	}
	usr := &UserModel{Email: email, HashedPassword: &hash}
	if createErr := s.userStore.Create(ctx, usr); createErr != nil {
		if errors.Is(createErr, ErrAlreadyExists) {
			return nil, status.Error(codes.AlreadyExists, "user already exists")
		}
		slog.ErrorContext(ctx, "failed to create user", "error", createErr, "email", email)
		return nil, status.Error(codes.Internal, "failed to create user")
	}

	s.recordLogin(ctx, usr)
	tokenPair, err := security.NewTokenPair(usr.ID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create token pair (register)", "error", err)
		return nil, status.Error(codes.Internal, "failed to create token pair")
	}

	return &identra_v1_pb.RegisterByPasswordResponse{Token: tokenPair}, nil
}

func (s *Service) LoginByPassword(
	ctx context.Context,
	req *identra_v1_pb.LoginByPasswordRequest,
) (*identra_v1_pb.LoginByPasswordResponse, error) {
	email := strings.TrimSpace(req.GetEmail())
	password := req.GetPassword()
	if email == "" || password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	if s.loginRateLimiter != nil {
		allowed, rlErr := rateLimitAllowed(ctx, s.loginRateLimiter, rateLimitKeys(ctx, "login", email))
		if rlErr != nil {
			slog.ErrorContext(ctx, "login rate limiter error", "error", rlErr)
			// fail open
		} else if !allowed {
			return nil, status.Error(codes.ResourceExhausted, "too many failed attempts, please try again later")
		}
	}

	usr, err := s.userStore.GetByEmail(ctx, email)
	switch {
	case err == nil:
		// user found — verify password below
	case errors.Is(err, ErrNotFound):
		return nil, status.Error(codes.NotFound, "user not found")
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	if usr.HashedPassword == nil {
		return nil, status.Error(codes.FailedPrecondition, "password login not set up for this account")
	}

	valid, verifyErr := security.VerifyPassword(password, *usr.HashedPassword)
	if verifyErr != nil {
		slog.ErrorContext(ctx, "password verification failed", "error", verifyErr)
		return nil, status.Error(codes.Internal, "failed to verify password")
	}
	if !valid {
		if s.loginRateLimiter != nil {
			if recordErr := recordRateLimit(ctx, s.loginRateLimiter, rateLimitKeys(ctx, "login", email)); recordErr != nil {
				slog.ErrorContext(ctx, "failed to record login failure", "error", recordErr)
			}
		}
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	if s.loginRateLimiter != nil {
		if resetErr := s.loginRateLimiter.Reset(ctx, emailRateLimitKey("login", email)); resetErr != nil {
			slog.ErrorContext(ctx, "failed to reset login rate limit", "error", resetErr)
		}
	}

	s.recordLogin(ctx, usr)
	tokenPair, err := security.NewTokenPair(usr.ID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create token pair", "error", err)
		return nil, status.Error(codes.Internal, "failed to create token pair")
	}

	return &identra_v1_pb.LoginByPasswordResponse{Token: tokenPair}, nil
}
