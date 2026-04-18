package service

import (
	"context"
	"errors"

	"trackapp-server/internal/models"
	"trackapp-server/internal/repository"
)

// UserService provides business logic related to user profile and settings.
type UserService struct {
	users repository.UserRepository
}

// NewUserService constructs a new UserService.
func NewUserService(users repository.UserRepository) *UserService {
	return &UserService{users: users}
}

// EnsureUser makes sure the user exists in persistence layer.
func (s *UserService) EnsureUser(ctx context.Context, userID string, language string) (*models.User, error) {
	if userID == "" {
		return nil, errors.New("userID is required")
	}
	user := &models.User{ID: userID, ClientLanguage: language}
	return s.users.CreateIfNotExists(ctx, user)
}

// GetUserProfile returns the detail of a user.
func (s *UserService) GetUserProfile(ctx context.Context, userID string) (*models.User, error) {
	if userID == "" {
		return nil, errors.New("userID is required")
	}
	return s.users.FindByID(ctx, userID)
}

// UpdateAvatar updates user's avatar URL.
func (s *UserService) UpdateAvatar(ctx context.Context, userID, avatarURL string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.AvatarURL = avatarURL
	return user, s.users.Update(ctx, user)
}

// UpdateName updates user's nickname.
func (s *UserService) UpdateName(ctx context.Context, userID, name string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.Nickname = name
	return user, s.users.Update(ctx, user)
}

// UpdateSignature updates user's signature.
func (s *UserService) UpdateSignature(ctx context.Context, userID, sig string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.Signature = sig
	return user, s.users.Update(ctx, user)
}

// UpdateClientLanguage updates user's client language.
func (s *UserService) UpdateClientLanguage(ctx context.Context, userID, lang string) (*models.User, error) {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.ClientLanguage = lang
	return user, s.users.Update(ctx, user)
}
