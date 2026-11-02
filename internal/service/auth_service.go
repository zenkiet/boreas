package service

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
	"golang.org/x/crypto/bcrypt"
)

const (
	tokenTTL          = 30 * 24 * time.Hour
	maxAPITokenTTL    = 90 * 24 * time.Hour
	minPasswordLength = 8
	pairingTTL        = 2 * time.Minute
)

type AuthService struct {
	users   core.UserStore
	tokens  core.TokenStore
	Avatars core.ObjectStore

	mu       sync.Mutex
	pairings map[string]pairing
}

type pairing struct {
	userID  uuid.UUID
	expires time.Time
	claimed chan struct{} // one value, then closed, once the code signs a device in
}

func NewAuthService(users core.UserStore, tokens core.TokenStore) *AuthService {
	return &AuthService{users: users, tokens: tokens, pairings: map[string]pairing{}}
}

func wrap(operation string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, err)
}

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

	token, err := s.session(ctx, user.ID)
	return token, user, err
}

func (s *AuthService) session(ctx context.Context, userID uuid.UUID) (string, error) {
	token := rand.Text()
	now := time.Now().UTC()
	if _, err := s.tokens.Create(ctx, core.AuthToken{
		UserID:    userID,
		Kind:      core.TokenKindSession,
		TokenHash: hashToken(token),
		ValidFrom: now,
		ExpiresAt: now.Add(tokenTTL),
	}); err != nil {
		return "", fmt.Errorf("persist token: %w", err)
	}
	return token, nil
}

// StartPairing returns a code that signs its user in on another device, once, within pairingTTL.
// ponytail: in memory, one instance; a table if Boreas ever runs replicas.
func (s *AuthService) StartPairing(user core.User) (string, time.Time) {
	code, now := rand.Text(), time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	maps.DeleteFunc(s.pairings, func(_ string, p pairing) bool { return now.After(p.expires) })
	s.pairings[hashToken(code)] = pairing{user.ID, now.Add(pairingTTL), make(chan struct{}, 1)}
	return code, now.Add(pairingTTL)
}

// WatchPairing hears the user's own code sign a device in; ErrNotFound once it is spent or expired.
func (s *AuthService) WatchPairing(user core.User, code string) (<-chan struct{}, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pairings[hashToken(code)]
	if !ok || p.userID != user.ID || time.Now().After(p.expires) {
		return nil, time.Time{}, core.ErrNotFound
	}
	return p.claimed, p.expires, nil
}

// ClaimPairing spends a code from StartPairing on a new login session.
func (s *AuthService) ClaimPairing(ctx context.Context, code string) (string, core.User, error) {
	s.mu.Lock()
	p, ok := s.pairings[hashToken(code)]
	delete(s.pairings, hashToken(code))
	s.mu.Unlock()
	if !ok || time.Now().After(p.expires) {
		return "", core.User{}, core.ErrUnauthorized
	}
	user, err := s.users.Get(ctx, p.userID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return "", core.User{}, core.ErrUnauthorized
		}
		return "", core.User{}, fmt.Errorf("lookup pairing user: %w", err)
	}
	if user.Disabled() {
		return "", core.User{}, core.ErrUnauthorized
	}
	token, err := s.session(ctx, user.ID)
	if err == nil {
		p.claimed <- struct{}{}
		close(p.claimed)
	}
	return token, user, err
}

func (s *AuthService) Authenticate(ctx context.Context, token string) (core.User, core.TokenKind, error) {
	if strings.TrimSpace(token) == "" {
		return core.User{}, "", core.ErrUnauthorized
	}
	stored, err := s.tokens.GetByHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return core.User{}, "", core.ErrUnauthorized
		}
		return core.User{}, "", fmt.Errorf("lookup token: %w", err)
	}
	now := time.Now().UTC()
	if stored.RevokedAt != nil || stored.ValidFrom.After(now) || !stored.ExpiresAt.After(now) {
		return core.User{}, "", core.ErrUnauthorized
	}
	user, err := s.users.Get(ctx, stored.UserID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return core.User{}, "", core.ErrUnauthorized
		}
		return core.User{}, "", fmt.Errorf("lookup token user: %w", err)
	}
	if user.Disabled() {
		return core.User{}, "", core.ErrUnauthorized
	}
	return user, stored.Kind, nil
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	return wrap("revoke token", s.tokens.Revoke(ctx, hashToken(token)))
}

type CreateAPITokenInput struct {
	Name      string
	ValidFrom time.Time
	ValidTo   time.Time
}

// CreateAPIToken returns the plaintext credential once; only its hash is persisted.
func (s *AuthService) CreateAPIToken(ctx context.Context, userID uuid.UUID, in CreateAPITokenInput) (string, core.AuthToken, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", core.AuthToken{}, errors.Join(core.ErrInvalidInput,
			errors.New("token name must be between 1 and 100 characters"))
	}
	validFrom, validTo := in.ValidFrom.UTC(), in.ValidTo.UTC()
	if in.ValidFrom.IsZero() || in.ValidTo.IsZero() {
		return "", core.AuthToken{}, errors.Join(core.ErrInvalidInput,
			errors.New("valid_from and valid_to are required"))
	}
	if !validFrom.Before(validTo) {
		return "", core.AuthToken{}, errors.Join(core.ErrInvalidInput,
			errors.New("valid_from must be before valid_to"))
	}
	if validTo.Sub(validFrom) > maxAPITokenTTL {
		return "", core.AuthToken{}, errors.Join(core.ErrInvalidInput,
			errors.New("API token validity must not exceed 90 days"))
	}
	if !validTo.After(time.Now().UTC()) {
		return "", core.AuthToken{}, errors.Join(core.ErrInvalidInput,
			errors.New("valid_to must be in the future"))
	}

	raw := rand.Text()
	created, err := s.tokens.Create(ctx, core.AuthToken{
		UserID: userID, Name: name, Kind: core.TokenKindAPI,
		TokenHash: hashToken(raw), ValidFrom: validFrom, ExpiresAt: validTo,
	})
	if err != nil {
		return "", core.AuthToken{}, fmt.Errorf("persist API token: %w", err)
	}
	return raw, created, nil
}

func (s *AuthService) ListAPITokens(ctx context.Context, userID uuid.UUID) ([]core.AuthToken, error) {
	tokens, err := s.tokens.ListAPITokens(ctx, userID)
	return tokens, wrap("list API tokens", err)
}

func (s *AuthService) RevokeAPIToken(ctx context.Context, userID, tokenID uuid.UUID) error {
	return wrap("revoke API token", s.tokens.RevokeByID(ctx, userID, tokenID))
}

func (s *AuthService) ListUsers(ctx context.Context) ([]core.User, error) {
	users, err := s.users.List(ctx)
	return users, wrap("list users", err)
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
	role := cmp.Or(in.Role, core.RoleUser)
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
	return user, wrap("create user", err)
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
			user.DisabledAt = new(time.Now())
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
		return wrap("delete user", err)
	}
	if s.Avatars != nil {
		// Best effort: an orphan only costs storage.
		_ = s.Avatars.Delete(ctx, avatarKey(id))
	}
	return nil
}

func (s *AuthService) SetAvatar(ctx context.Context, actor core.User, mime string, body []byte) (core.User, error) {
	if s.Avatars == nil {
		return core.User{}, core.ErrConflict
	}
	if !slices.Contains(boraImageTypes, mime) {
		return core.User{}, errors.Join(core.ErrInvalidInput, errors.New("image must be JPEG, PNG, WebP or GIF"))
	}
	if err := s.Avatars.Put(ctx, avatarKey(actor.ID), mime, body); err != nil {
		return core.User{}, wrap("store avatar", err)
	}
	user, err := s.users.SetAvatar(ctx, actor.ID)
	return user, wrap("set avatar", err)
}

func (s *AuthService) Avatar(ctx context.Context, id uuid.UUID) ([]byte, string, error) {
	if s.Avatars == nil {
		return nil, "", core.ErrNotFound
	}
	body, mime, err := s.Avatars.Get(ctx, avatarKey(id))
	return body, mime, wrap("get avatar", err)
}

func avatarKey(id uuid.UUID) string { return "avatars/" + id.String() }

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
