// Package auth implements password verification, session lifecycle and login throttling.
package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"siracrm/internal/auth/token"
	"siracrm/internal/domain"
	"siracrm/internal/store"
)

const (
	PasswordMinBytes = 14
	PasswordMaxBytes = 72
	PasswordCost     = 12
	SessionLifetime  = 12 * time.Hour
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrRateLimited = errors.New("too many attempts; try again later")

// A valid cost-12 hash keeps unknown-email verification comparable to known accounts.
const dummyPasswordHash = "$2a$12$R9h/cIPz0gi.URNNX3kh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW"

type Service struct {
	repository *store.Repository
	limiter    *loginLimiter
}

func New(repository *store.Repository) *Service {
	return &Service{repository: repository, limiter: newLoginLimiter()}
}
func (service *Service) Bootstrap(ctx context.Context, email, password string) error {
	exists, err := service.repository.HasUsers(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return errors.New("first startup requires a valid ADMIN_EMAIL")
	}
	if err = validatePassword(password); err != nil {
		return err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	if err != nil {
		return err
	}
	return service.repository.BootstrapAdmin(ctx, email, string(passwordHash))
}
func (service *Service) Login(ctx context.Context, email, password string) (string, error) {
	if len(email) > 254 || len(password) > PasswordMaxBytes {
		return "", ErrInvalidCredentials
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !service.limiter.allow(email) {
		return "", ErrRateLimited
	}
	credentials, lookupErr := service.repository.CredentialsByEmail(ctx, email)
	passwordHash := credentials.PasswordHash
	if lookupErr != nil {
		passwordHash = dummyPasswordHash
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
	if lookupErr != nil && !errors.Is(lookupErr, store.ErrNotFound) {
		return "", lookupErr
	}
	if lookupErr != nil || passwordErr != nil {
		return "", ErrInvalidCredentials
	}
	sessionToken := token.New()
	if err := service.repository.CreateSession(ctx, credentials.UserID, sessionToken, time.Now().Add(SessionLifetime)); err != nil {
		return "", err
	}
	service.limiter.reset(email)
	return sessionToken, nil
}
func (service *Service) SessionUser(ctx context.Context, sessionToken string) (store.Identity, error) {
	return service.repository.SessionUser(ctx, sessionToken)
}
func (service *Service) Logout(ctx context.Context, sessionToken string) error {
	return service.repository.DeleteSession(ctx, sessionToken)
}
func (service *Service) NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return "", domain.Invalid("Enter a valid email address")
	}
	return email, nil
}
func (service *Service) AcceptInvite(ctx context.Context, plainToken, name, password string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return "", domain.Invalid("Name must contain 1–80 characters")
	}
	if err := validatePassword(password); err != nil {
		return "", err
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), PasswordCost)
	if err != nil {
		return "", err
	}
	sessionToken := token.New()
	_, err = service.repository.AcceptInvite(ctx, plainToken, name, string(passwordHash), sessionToken, time.Now().Add(SessionLifetime))
	if err != nil {
		return "", err
	}
	return sessionToken, nil
}
func (service *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	credentials, err := service.repository.CredentialsByID(ctx, userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(credentials.PasswordHash), []byte(currentPassword)) != nil {
		return domain.Invalid("Current password is incorrect")
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), PasswordCost)
	if err != nil {
		return err
	}
	return service.repository.ReplacePassword(ctx, userID, credentials.PasswordHash, string(passwordHash))
}
func validatePassword(password string) error {
	if len(password) < PasswordMinBytes || len(password) > PasswordMaxBytes {
		return domain.Invalid("Password must be 14–72 bytes")
	}
	return nil
}
