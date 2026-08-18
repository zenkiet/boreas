package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zenkiet/boreas/internal/core"
)

func newAuth(t *testing.T) (*AuthService, *fakeUserStore, *fakeTokenStore) {
	t.Helper()
	users, tokens := newFakeUserStore(), newFakeTokenStore()
	auth, err := NewAuthService(users, tokens)
	if err != nil {
		t.Fatal(err)
	}
	return auth, users, tokens
}

func TestLoginAndAuthenticate(t *testing.T) {
	auth, _, _ := newAuth(t)
	created, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "alice", Email: "alice@example.com", Password: "correct-horse", Role: core.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.PasswordHash == "correct-horse" {
		t.Fatal("password was stored in plaintext")
	}

	token, user, err := auth.Login(context.Background(), "ALICE", "correct-horse")
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || user.ID != created.ID {
		t.Fatalf("unexpected login result: %q %+v", token, user)
	}

	authenticated, kind, err := auth.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated.ID != created.ID {
		t.Fatal("authenticate returned the wrong user")
	}
	if kind != core.TokenKindSession {
		t.Fatalf("token kind = %q", kind)
	}
}

func TestLoginSessionExpiresAfterThirtyDays(t *testing.T) {
	auth, _, tokens := newAuth(t)
	if _, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "session", Email: "session@example.com", Password: "password123",
	}); err != nil {
		t.Fatal(err)
	}
	raw, _, err := auth.Login(context.Background(), "session", "password123")
	if err != nil {
		t.Fatal(err)
	}
	stored := tokens.tokens[hashToken(raw)]
	if stored.Kind != core.TokenKindSession || stored.ExpiresAt.Sub(stored.ValidFrom) != tokenTTL {
		t.Fatalf("unexpected login token: %+v", stored)
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	auth, _, _ := newAuth(t)
	if _, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "bob", Email: "bob@example.com", Password: "password123",
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Login(context.Background(), "bob", "wrong"); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
	if _, _, err := auth.Login(context.Background(), "nobody", "password123"); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
}

func TestTokensAreStoredHashed(t *testing.T) {
	auth, _, tokens := newAuth(t)
	if _, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "carol", Email: "c@example.com", Password: "password123",
	}); err != nil {
		t.Fatal(err)
	}
	token, _, err := auth.Login(context.Background(), "carol", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, plaintextStored := tokens.tokens[token]; plaintextStored {
		t.Fatal("the plaintext token was stored")
	}
	if _, hashed := tokens.tokens[hashToken(token)]; !hashed {
		t.Fatal("the token hash was not stored")
	}
}

func TestAuthenticateRejectsRevokedExpiredAndDisabled(t *testing.T) {
	auth, users, tokens := newAuth(t)
	user, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "dave", Email: "d@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatal(err)
	}

	revoked, _, err := auth.Login(context.Background(), "dave", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Logout(context.Background(), revoked); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Authenticate(context.Background(), revoked); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("revoked token accepted: %v", err)
	}

	expired, _, err := auth.Login(context.Background(), "dave", "password123")
	if err != nil {
		t.Fatal(err)
	}
	stored := tokens.tokens[hashToken(expired)]
	stored.ExpiresAt = time.Now().Add(-time.Minute)
	tokens.tokens[hashToken(expired)] = stored
	if _, _, err := auth.Authenticate(context.Background(), expired); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("expired token accepted: %v", err)
	}

	active, _, err := auth.Login(context.Background(), "dave", "password123")
	if err != nil {
		t.Fatal(err)
	}
	disabled := users.users[user.ID]
	now := time.Now()
	disabled.DisabledAt = &now
	users.users[user.ID] = disabled
	if _, _, err := auth.Authenticate(context.Background(), active); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("token for a disabled user accepted: %v", err)
	}
	if _, _, err := auth.Login(context.Background(), "dave", "password123"); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("disabled user could log in: %v", err)
	}
}

func TestPasswordChangeRevokesExistingTokens(t *testing.T) {
	auth, _, _ := newAuth(t)
	user, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "erin", Email: "e@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := auth.Login(context.Background(), "erin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	newPassword := "new-password-1"
	if _, err := auth.UpdateUser(context.Background(), user.ID, UpdateUserInput{Password: &newPassword}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Authenticate(context.Background(), token); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("old token still valid after a password change: %v", err)
	}
}

func TestCreateListAuthenticateAndRevokeAPIToken(t *testing.T) {
	auth, _, tokens := newAuth(t)
	user, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "deployer", Email: "deploy@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatal(err)
	}
	validFrom := time.Now().UTC().Add(-time.Minute)
	validTo := validFrom.Add(24 * time.Hour)
	raw, created, err := auth.CreateAPIToken(context.Background(), user.ID, CreateAPITokenInput{
		Name: " staging ", ValidFrom: validFrom, ValidTo: validTo,
	})
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" || created.ID == uuid.Nil || created.Name != "staging" || created.Kind != core.TokenKindAPI {
		t.Fatalf("unexpected token: raw=%q metadata=%+v", raw, created)
	}
	if created.TokenHash == raw {
		t.Fatal("plaintext API token was persisted")
	}
	if _, plaintextStored := tokens.tokens[raw]; plaintextStored {
		t.Fatal("plaintext API token was used as a storage key")
	}
	if _, hashed := tokens.tokens[hashToken(raw)]; !hashed {
		t.Fatal("API token hash was not stored")
	}

	authenticated, kind, err := auth.Authenticate(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	if authenticated.ID != user.ID || kind != core.TokenKindAPI {
		t.Fatalf("unexpected authentication: user=%+v kind=%q", authenticated, kind)
	}

	listed, err := auth.ListAPITokens(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed tokens = %+v", listed)
	}

	if err := auth.RevokeAPIToken(context.Background(), user.ID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Authenticate(context.Background(), raw); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("revoked API token accepted: %v", err)
	}
}

func TestScheduledAPITokenCannotAuthenticateYet(t *testing.T) {
	auth, _, _ := newAuth(t)
	user, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "future", Email: "future@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatal(err)
	}
	validFrom := time.Now().UTC().Add(time.Hour)
	raw, _, err := auth.CreateAPIToken(context.Background(), user.ID, CreateAPITokenInput{
		Name: "future", ValidFrom: validFrom, ValidTo: validFrom.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Authenticate(context.Background(), raw); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("scheduled API token accepted: %v", err)
	}
}

func TestCreateAPITokenValidation(t *testing.T) {
	auth, _, _ := newAuth(t)
	userID := uuid.New()
	now := time.Now().UTC()
	valid := CreateAPITokenInput{Name: "ci", ValidFrom: now, ValidTo: now.Add(time.Hour)}
	cases := map[string]CreateAPITokenInput{
		"empty name":       {Name: "  ", ValidFrom: valid.ValidFrom, ValidTo: valid.ValidTo},
		"long name":        {Name: strings.Repeat("x", 101), ValidFrom: valid.ValidFrom, ValidTo: valid.ValidTo},
		"missing from":     {Name: "ci", ValidTo: valid.ValidTo},
		"missing to":       {Name: "ci", ValidFrom: valid.ValidFrom},
		"reversed":         {Name: "ci", ValidFrom: valid.ValidTo, ValidTo: valid.ValidFrom},
		"same instant":     {Name: "ci", ValidFrom: valid.ValidFrom, ValidTo: valid.ValidFrom},
		"over ninety days": {Name: "ci", ValidFrom: now, ValidTo: now.Add(maxAPITokenTTL + time.Second)},
		"already expired":  {Name: "ci", ValidFrom: now.Add(-2 * time.Hour), ValidTo: now.Add(-time.Hour)},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := auth.CreateAPIToken(context.Background(), userID, in); !errors.Is(err, core.ErrInvalidInput) {
				t.Fatalf("got %v, want ErrInvalidInput", err)
			}
		})
	}

	valid.ValidTo = valid.ValidFrom.Add(maxAPITokenTTL)
	if _, _, err := auth.CreateAPIToken(context.Background(), userID, valid); err != nil {
		t.Fatalf("exactly 90 days must be accepted: %v", err)
	}
}

func TestUserCannotRevokeAnotherUsersAPIToken(t *testing.T) {
	auth, _, _ := newAuth(t)
	now := time.Now().UTC()
	_, token, err := auth.CreateAPIToken(context.Background(), uuid.New(), CreateAPITokenInput{
		Name: "owner", ValidFrom: now, ValidTo: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.RevokeAPIToken(context.Background(), uuid.New(), token.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestPasswordChangeRevokesAPITokens(t *testing.T) {
	auth, _, _ := newAuth(t)
	user, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "robot", Email: "robot@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	raw, _, err := auth.CreateAPIToken(context.Background(), user.ID, CreateAPITokenInput{
		Name: "build", ValidFrom: now.Add(-time.Minute), ValidTo: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	newPassword := "new-password-1"
	if _, err := auth.UpdateUser(context.Background(), user.ID, UpdateUserInput{Password: &newPassword}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := auth.Authenticate(context.Background(), raw); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("API token survived password change: %v", err)
	}
}

func TestCreateUserValidation(t *testing.T) {
	auth, _, _ := newAuth(t)
	cases := map[string]CreateUserInput{
		"no username": {Email: "a@b.c", Password: "password123"},
		"no email":    {Username: "x", Password: "password123"},
		"short":       {Username: "x", Email: "a@b.c", Password: "short"},
		"bad role":    {Username: "x", Email: "a@b.c", Password: "password123", Role: "superuser"},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := auth.CreateUser(context.Background(), in); !errors.Is(err, core.ErrInvalidInput) {
				t.Fatalf("got %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestEnsureAdminOnlySeedsEmptyDatabase(t *testing.T) {
	auth, users, _ := newAuth(t)
	created, err := auth.EnsureAdmin(context.Background(), "root", "root@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if !created || len(users.users) != 1 {
		t.Fatal("admin was not seeded")
	}
	for _, user := range users.users {
		if user.Role != core.RoleAdmin {
			t.Fatalf("seeded user is not an admin: %+v", user)
		}
	}

	created, err = auth.EnsureAdmin(context.Background(), "root2", "root2@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if created || len(users.users) != 1 {
		t.Fatal("a second admin was seeded into a non-empty database")
	}
}

func TestAuthenticateRejectsUnknownToken(t *testing.T) {
	auth, _, _ := newAuth(t)
	if _, _, err := auth.Authenticate(context.Background(), ""); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("empty token: got %v", err)
	}
	if _, _, err := auth.Authenticate(context.Background(), "made-up"); !errors.Is(err, core.ErrUnauthorized) {
		t.Fatalf("unknown token: got %v", err)
	}
}

func TestDeleteUser(t *testing.T) {
	auth, _, _ := newAuth(t)
	user, err := auth.CreateUser(context.Background(), CreateUserInput{
		Username: "frank", Email: "f@example.com", Password: "password123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.DeleteUser(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	if err := auth.DeleteUser(context.Background(), uuid.New()); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
