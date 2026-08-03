package db

import (
	"testing"
	"time"
)

func createTestUser(t *testing.T, email string) *User {
	t.Helper()
	user, err := Connection().CreateUser(&User{
		Email:        email,
		PasswordHash: "not-a-real-hash",
		Active:       true,
	})
	if err != nil {
		t.Fatalf("creating user: %v", err)
	}
	t.Cleanup(func() { Connection().db.Unscoped().Delete(&User{}, "id = ?", user.ID) })
	return user
}

func TestTouchUserLastLoginSetsTimestamp(t *testing.T) {
	user := createTestUser(t, "touch-last-login@example.com")

	if user.LastLoginAt != nil {
		t.Fatalf("new user LastLoginAt = %v, want nil", user.LastLoginAt)
	}

	before := time.Now().Add(-time.Second)
	if err := Connection().TouchUserLastLogin(user.ID); err != nil {
		t.Fatalf("TouchUserLastLogin: %v", err)
	}

	reloaded, err := Connection().GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if reloaded.LastLoginAt == nil {
		t.Fatal("LastLoginAt = nil, want a timestamp")
	}
	if reloaded.LastLoginAt.Before(before) {
		t.Errorf("LastLoginAt = %v, want at or after %v", reloaded.LastLoginAt, before)
	}
}

func TestNewUserDefaultsToNotSuperuser(t *testing.T) {
	user := createTestUser(t, "default-not-superuser@example.com")

	reloaded, err := Connection().GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if reloaded.Superuser {
		t.Error("Superuser = true, want false by default")
	}
}
