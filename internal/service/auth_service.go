package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
	"golang.org/x/crypto/bcrypt"
)

const tokenTTL = 30 * 24 * time.Hour

const minPasswordLength = 8

type AuthService struct {
	users  core.UserStore
	tokens core.TokenStore
}

func NewAuthService(users core.UserStore, tokens core.TokenStore) (*AuthService, error) {
	if users == nil || tokens == nil {
		return nil, errors.Join(core.ErrInvalidInput, errors.New("user and token stores are required"))
	}
	return &AuthService{users: users, tokens: tokens}, nil
}

// hashToken derives a one-way representation so leaked storage cannot reveal bearer tokens.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Login returns a plaintext token once and never stores it.
func (s *AuthService) Login(ctx context.Context, username, password string) (string, core.User, error) {
	user, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			// Use a dummy comparison to reduce username-enumeration timing differences.
			_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$"+strings.Repeat("x", 53)), []byte(password))
			return "", core.User{}, core.ErrUnauthorized
		}
		return "", core.User{}, fmt.Errorf("lookup user: %w", err)
	}
	if user.Disabled() {
		return "", core.User{}, core.ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", core.User{}, core.ErrUnauthorized
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", core.User{}, fmt.Errorf("generate token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.tokens.Create(ctx, core.AuthToken{
		UserID:    user.ID,
		TokenHash: hashToken(token),
		ExpiresAt: time.Now().Add(tokenTTL),
	}); err != nil {
		return "", core.User{}, fmt.Errorf("persist token: %w", err)
	}
	return token, user, nil
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (core.User, error) {
	if strings.TrimSpace(token) == "" {
		return core.User{}, core.ErrUnauthorized
	}
	stored, err := s.tokens.GetByHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return core.User{}, core.ErrUnauthorized
		}
		return core.User{}, fmt.Errorf("lookup token: %w", err)
	}
	if stored.RevokedAt != nil || !stored.ExpiresAt.After(time.Now()) {
		return core.User{}, core.ErrUnauthorized
	}
	user, err := s.users.Get(ctx, stored.UserID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return core.User{}, core.ErrUnauthorized
		}
		return core.User{}, fmt.Errorf("lookup token user: %w", err)
	}
	if user.Disabled() {
		return core.User{}, core.ErrUnauthorized
	}
	return user, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	if err := s.tokens.Revoke(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

func (s *AuthService) ListUsers(ctx context.Context) ([]core.User, error) {
	users, err := s.users.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

type CreateUserInput struct {
	Username string
	Email    string
	Password string
	Role     core.UserRole
}

func (s *AuthService) CreateUser(ctx context.Context, in CreateUserInput) (core.User, error) {
	if strings.TrimSpace(in.Username) == "" {
		return core.User{}, errors.Join(core.ErrInvalidInput, errors.New("username is required"))
	}
	if strings.TrimSpace(in.Email) == "" {
		return core.User{}, errors.Join(core.ErrInvalidInput, errors.New("email is required"))
	}
	if len(in.Password) < minPasswordLength {
		return core.User{}, errors.Join(core.ErrInvalidInput,
			fmt.Errorf("password must be at least %d characters", minPasswordLength))
	}
	role := in.Role
	if role == "" {
		role = core.RoleUser
	}
	if role != core.RoleAdmin && role != core.RoleUser {
		return core.User{}, errors.Join(core.ErrInvalidInput, errors.New("role must be admin or user"))
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return core.User{}, fmt.Errorf("hash password: %w", err)
	}
	user, err := s.users.Create(ctx, core.User{
		Username: in.Username, Email: in.Email, PasswordHash: string(hash), Role: role,
	})
	if err != nil {
		return core.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

type UpdateUserInput struct {
	Email    *string
	Password *string
	Role     *core.UserRole
	Disabled *bool
}

func (s *AuthService) UpdateUser(ctx context.Context, id uuid.UUID, in UpdateUserInput) (core.User, error) {
	user, err := s.users.Get(ctx, id)
	if err != nil {
		return core.User{}, fmt.Errorf("get user: %w", err)
	}
	if in.Email != nil {
		if strings.TrimSpace(*in.Email) == "" {
			return core.User{}, errors.Join(core.ErrInvalidInput, errors.New("email must not be empty"))
		}
		user.Email = *in.Email
	}
	if in.Password != nil {
		if len(*in.Password) < minPasswordLength {
			return core.User{}, errors.Join(core.ErrInvalidInput,
				fmt.Errorf("password must be at least %d characters", minPasswordLength))
		}
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(*in.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			return core.User{}, fmt.Errorf("hash password: %w", hashErr)
		}
		user.PasswordHash = string(hash)
	}
	if in.Role != nil {
		if *in.Role != core.RoleAdmin && *in.Role != core.RoleUser {
			return core.User{}, errors.Join(core.ErrInvalidInput, errors.New("role must be admin or user"))
		}
		user.Role = *in.Role
	}
	if in.Disabled != nil {
		if *in.Disabled && user.DisabledAt == nil {
			now := time.Now()
			user.DisabledAt = &now
		} else if !*in.Disabled {
			user.DisabledAt = nil
		}
	}
	updated, err := s.users.Update(ctx, user)
	if err != nil {
		return core.User{}, fmt.Errorf("update user: %w", err)
	}
	// Revoke stale access after credential or privilege changes.
	if in.Password != nil || in.Role != nil || (in.Disabled != nil && *in.Disabled) {
		if err := s.tokens.RevokeAllForUser(ctx, updated.ID); err != nil {
			return core.User{}, fmt.Errorf("revoke user tokens: %w", err)
		}
	}
	return updated, nil
}

func (s *AuthService) DeleteUser(ctx context.Context, id uuid.UUID) error {
	if err := s.users.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// EnsureAdmin seeds only an empty installation to avoid creating extra privileged users.
func (s *AuthService) EnsureAdmin(ctx context.Context, username, email, password string) (bool, error) {
	count, err := s.users.Count(ctx)
	if err != nil {
		return false, fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return false, nil
	}
	if _, err := s.CreateUser(ctx, CreateUserInput{
		Username: username, Email: email, Password: password, Role: core.RoleAdmin,
	}); err != nil {
		return false, err
	}
	return true, nil
}
