package identra

import (
	"context"
	"fmt"

	"github.com/google/go-github/v73/github"
)

type UserInfo struct {
	Provider  string
	ID        string
	Email     string
	Username  string
	AvatarURL string
}

type UserInfoProvider interface {
	GetUserInfo(ctx context.Context, token string) (UserInfo, error)
}

type EmailProvider interface {
	GetEmail(ctx context.Context, token string) (string, error)
}

func GetUserProvider(name string) (UserInfoProvider, error) {
	switch name {
	case "github":
		return &GitHubUserInfoProvider{}, nil
	default:
		return nil, fmt.Errorf("provider %s not supported", name)
	}
}

type GitHubUserInfoProvider struct{}

func (g *GitHubUserInfoProvider) GetUserInfo(ctx context.Context, token string) (UserInfo, error) {
	client := github.NewClient(nil).WithAuthToken(token)
	user, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return UserInfo{}, err
	}
	return UserInfo{
		Provider:  "github",
		ID:        fmt.Sprintf("%d", user.GetID()),
		Email:     user.GetEmail(),
		Username:  user.GetLogin(),
		AvatarURL: user.GetAvatarURL(),
	}, nil
}

func (g *GitHubUserInfoProvider) GetEmail(ctx context.Context, token string) (string, error) {
	client := github.NewClient(nil).WithAuthToken(token)
	emails, _, err := client.Users.ListEmails(ctx, nil)
	if err != nil {
		return "", err
	}

	var firstVerified string
	var firstAny string
	for _, e := range emails {
		email := e.GetEmail()
		if email == "" {
			continue
		}
		if firstAny == "" {
			firstAny = email
		}
		if e.GetPrimary() && e.GetVerified() {
			return email, nil
		}
		if firstVerified == "" && e.GetVerified() {
			firstVerified = email
		}
	}

	if firstVerified != "" {
		return firstVerified, nil
	}
	return firstAny, nil
}
