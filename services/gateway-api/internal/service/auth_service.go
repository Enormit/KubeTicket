package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/Enormit/kubeticket/services/gateway-api/internal/repo"
	"github.com/Enormit/kubeticket/services/pkg/middleware"
	"github.com/Enormit/kubeticket/services/pkg/models"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrInvalidCredentials = errors.New("invalid email or password")
)

// AuthService manages user registration, login, and JWT issuance.
type AuthService struct {
	repo      repo.AuthRepository
	jwtSecret string
	jwtTTL    time.Duration
}

// NewAuthService creates a new AuthService.
func NewAuthService(r repo.AuthRepository, jwtSecret string, jwtTTL time.Duration) *AuthService {
	return &AuthService{repo: r, jwtSecret: jwtSecret, jwtTTL: jwtTTL}
}

// Register validates credentials, hashes password with bcrypt, and returns user + JWT.
func (s *AuthService) Register(ctx context.Context, email, password, fullName string) (*models.User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	fullName = strings.TrimSpace(fullName)
	if _, err := mail.ParseAddress(email); err != nil || len(password) < 6 || fullName == "" {
		return nil, "", ErrInvalidInput
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}

	u, err := s.repo.CreateUser(ctx, email, string(hash), fullName)
	if err != nil {
		return nil, "", err
	}

	tok, err := middleware.GenerateToken(s.jwtSecret, s.jwtTTL, u.ID, u.Email, u.Role)
	if err != nil {
		return nil, "", err
	}
	return u, tok, nil
}

// Login verifies email/password and returns user + signed JWT.
func (s *AuthService) Login(ctx context.Context, email, password string) (*models.User, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil, "", ErrInvalidInput
	}

	u, hash, err := s.repo.GetUserByEmail(ctx, email)
	if errors.Is(err, repo.ErrNotFound) {
		return nil, "", ErrInvalidCredentials
	}
	if err != nil {
		return nil, "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	tok, err := middleware.GenerateToken(s.jwtSecret, s.jwtTTL, u.ID, u.Email, u.Role)
	if err != nil {
		return nil, "", err
	}
	return u, tok, nil
}

// GetMe returns the user record for an authenticated ID.
func (s *AuthService) GetMe(ctx context.Context, userID string) (*models.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}
