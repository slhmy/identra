package identra

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"

	identra_v1_pb "github.com/slhmy/identra/gen/go/identra/v1"
	"github.com/slhmy/identra/internal/security"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) RequestEmailLoginCode(
	ctx context.Context,
	req *identra_v1_pb.RequestEmailLoginCodeRequest,
) (*identra_v1_pb.RequestEmailLoginCodeResponse, error) {
	if s.mailer == nil {
		return nil, status.Error(codes.FailedPrecondition, "mail service is disabled")
	}

	email := strings.TrimSpace(req.GetEmail())
	if email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}

	if s.sendCodeRateLimiter != nil {
		keys := rateLimitKeys(ctx, "send_code", email)
		allowed, rlErr := rateLimitAllowed(ctx, s.sendCodeRateLimiter, keys)
		if rlErr != nil {
			slog.ErrorContext(ctx, "send-code rate limiter error", "error", rlErr)
			// fail open — a limiter error must not prevent legitimate users
		} else if !allowed {
			return nil, status.Error(codes.ResourceExhausted, "too many verification code requests, please try again later")
		}
		if rlErr == nil {
			if recordErr := recordRateLimit(ctx, s.sendCodeRateLimiter, keys); recordErr != nil {
				slog.ErrorContext(ctx, "failed to record send-code attempt", "error", recordErr)
			}
		}
	}

	code, err := generateEmailCode()
	if err != nil {
		slog.ErrorContext(ctx, "failed to generate email code", "error", err)
		return nil, status.Error(codes.Internal, "failed to generate code")
	}

	const expiryMinutes = 10
	if err := s.emailCodeStore.Set(ctx, email, code); err != nil {
		slog.ErrorContext(ctx, "failed to store verification code", "error", err)
		return nil, status.Error(codes.Internal, "failed to store verification code")
	}

	if err := s.sendVerificationCode(email, code, expiryMinutes, req.GetUseHtml()); err != nil {
		slog.ErrorContext(ctx, "failed to send verification email", "error", err)
		return nil, status.Error(codes.Internal, "failed to send verification email")
	}

	return &identra_v1_pb.RequestEmailLoginCodeResponse{}, nil
}

func (s *Service) sendVerificationCode(to string, code string, expiryMinutes int, useHTML bool) error {
	subject := "Your Verification Code"
	if useHTML {
		htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<meta charset="UTF-8">
	<style>
		body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
		.container { max-width: 600px; margin: 0 auto; padding: 20px; }
		.header { background-color: #4CAF50; color: white; padding: 20px; text-align: center; }
		.content { background-color: #f9f9f9; padding: 30px; border-radius: 5px; margin-top: 20px; }
		.code { font-size: 32px; font-weight: bold; color: #4CAF50; text-align: center; letter-spacing: 5px; padding: 20px; background-color: #fff; border-radius: 5px; margin: 20px 0; }
		.footer { text-align: center; margin-top: 20px; color: #666; font-size: 12px; }
	</style>
</head>
<body>
	<div class="container">
		<div class="header">
			<h1>Verification Code</h1>
		</div>
		<div class="content">
			<p>Hello,</p>
			<p>You have requested a verification code. Please use the code below to complete your verification:</p>
			<div class="code">%s</div>
			<p>This code will expire in <strong>%d minutes</strong>.</p>
			<p>If you did not request this code, please ignore this email.</p>
		</div>
		<div class="footer">
			<p>This is an automated message, please do not reply.</p>
		</div>
	</div>
</body>
</html>
`, code, expiryMinutes)

		return s.mailer.SendEmail(EmailMessage{
			ToEmails: []string{to},
			Subject:  subject,
			Body:     htmlBody,
			IsHTML:   true,
		})
	}

	body := fmt.Sprintf("Your verification code is: %s (valid for %d minutes)", code, expiryMinutes)
	return s.mailer.SendEmail(EmailMessage{
		ToEmails: []string{to},
		Subject:  subject,
		Body:     body,
		IsHTML:   false,
	})
}

func (s *Service) LoginWithEmailCode(
	ctx context.Context,
	req *identra_v1_pb.LoginWithEmailCodeRequest,
) (*identra_v1_pb.LoginWithEmailCodeResponse, error) {
	email := strings.TrimSpace(req.GetEmail())
	code := strings.TrimSpace(req.GetCode())
	if email == "" || code == "" {
		return nil, status.Error(codes.InvalidArgument, "email and code are required")
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

	ok, err := s.emailCodeStore.Consume(ctx, email, code)
	if err != nil {
		slog.ErrorContext(ctx, "failed to validate verification code", "error", err)
		return nil, status.Error(codes.Internal, "failed to validate code")
	}
	if !ok {
		if s.loginRateLimiter != nil {
			if recordErr := recordRateLimit(ctx, s.loginRateLimiter, rateLimitKeys(ctx, "login", email)); recordErr != nil {
				slog.ErrorContext(ctx, "failed to record login failure", "error", recordErr)
			}
		}
		return nil, status.Error(codes.Unauthenticated, "invalid or expired code")
	}

	usr, err := s.userStore.GetByEmail(ctx, email)
	switch {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		usr = &UserModel{Email: email}
		if createErr := s.userStore.Create(ctx, usr); createErr != nil {
			return nil, status.Error(codes.Internal, "failed to create user")
		}
	default:
		return nil, status.Error(codes.Internal, "failed to fetch user")
	}

	if s.loginRateLimiter != nil {
		if resetErr := s.loginRateLimiter.Reset(ctx, emailRateLimitKey("login", email)); resetErr != nil {
			slog.ErrorContext(ctx, "failed to reset login rate limit", "error", resetErr)
		}
	}

	s.recordLogin(ctx, usr)
	tokenPair, err := security.NewTokenPair(usr.ID, s.tokenCfg)
	if err != nil {
		slog.ErrorContext(ctx, "failed to create token pair (email code)", "error", err)
		return nil, status.Error(codes.Internal, "failed to create token pair")
	}

	return &identra_v1_pb.LoginWithEmailCodeResponse{Tokens: tokenPair}, nil
}

func generateEmailCode() (string, error) {
	num, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", num.Int64()), nil
}
