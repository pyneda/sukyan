package db

import (
	"testing"
	"time"
)

func seedRosterUser(t *testing.T, email string, active, superuser bool, lastLogin *time.Time) *User {
	t.Helper()
	user, err := Connection().CreateUser(&User{
		Email:        email,
		PasswordHash: "not-a-real-hash",
		Active:       active,
		Superuser:    superuser,
		LastLoginAt:  lastLogin,
	})
	if err != nil {
		t.Fatalf("creating %s: %v", email, err)
	}
	t.Cleanup(func() { Connection().db.Unscoped().Delete(&User{}, "id = ?", user.ID) })
	return user
}

func ago(d time.Duration) *time.Time {
	at := time.Now().Add(-d)
	return &at
}

// Every seeded address shares this prefix so the assertions can filter down to
// this test's own rows regardless of what else the database holds.
const rosterPrefix = "roster-fixture-"

func seedRoster(t *testing.T) {
	t.Helper()
	seedRosterUser(t, rosterPrefix+"recent@example.com", true, true, ago(2*time.Hour))
	seedRosterUser(t, rosterPrefix+"week@example.com", true, false, ago(6*24*time.Hour))
	seedRosterUser(t, rosterPrefix+"dormant@example.com", true, false, ago(120*24*time.Hour))
	seedRosterUser(t, rosterPrefix+"never@example.com", true, false, nil)
	seedRosterUser(t, rosterPrefix+"disabled@example.com", false, false, ago(30*24*time.Hour))
}

func rosterFilter() UserListFilter {
	return UserListFilter{
		Query:      rosterPrefix,
		Pagination: Pagination{Page: 1, PageSize: 50},
	}
}

func TestListUsersFiltersAndCounts(t *testing.T) {
	seedRoster(t)

	rows, count, _, err := Connection().ListUsers(rosterFilter())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if count != 5 {
		t.Errorf("count = %d, want 5", count)
	}
	if len(rows) != 5 {
		t.Errorf("len(rows) = %d, want 5", len(rows))
	}
}

func TestListUsersSummaryClassifiesEveryState(t *testing.T) {
	seedRoster(t)

	_, _, summary, err := Connection().ListUsers(rosterFilter())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	if summary.SignedInLast7d < 2 {
		t.Errorf("SignedInLast7d = %d, want at least 2", summary.SignedInLast7d)
	}
	if summary.NeverSignedIn < 1 {
		t.Errorf("NeverSignedIn = %d, want at least 1", summary.NeverSignedIn)
	}
	if summary.Dormant < 1 {
		t.Errorf("Dormant = %d, want at least 1", summary.Dormant)
	}
	if summary.Disabled < 1 {
		t.Errorf("Disabled = %d, want at least 1", summary.Disabled)
	}
	if summary.Superusers < 1 {
		t.Errorf("Superusers = %d, want at least 1", summary.Superusers)
	}
	if summary.Total != summary.Active+summary.Disabled {
		t.Errorf("Total = %d, want Active(%d) + Disabled(%d)", summary.Total, summary.Active, summary.Disabled)
	}
}

// The summary describes the whole deployment, so it must not shrink when the
// caller narrows the list with a search.
func TestListUsersSummaryIgnoresTheQuery(t *testing.T) {
	seedRoster(t)

	_, unfilteredCount, wide, err := Connection().ListUsers(UserListFilter{
		Pagination: Pagination{Page: 1, PageSize: 1},
	})
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	_, narrowCount, narrow, err := Connection().ListUsers(rosterFilter())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}

	if narrowCount >= unfilteredCount {
		t.Fatalf("filtered count = %d, want fewer than %d", narrowCount, unfilteredCount)
	}
	if narrow.Total != wide.Total {
		t.Errorf("summary.Total = %d under a query, want %d", narrow.Total, wide.Total)
	}
}

// Never-signed-in accounts must never displace real activity at the top.
func TestListUsersSortsNullLastLoginLast(t *testing.T) {
	seedRoster(t)

	filter := rosterFilter()
	filter.SortBy = "last_login"
	filter.SortOrder = "desc"

	rows, _, _, err := Connection().ListUsers(filter)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("no rows returned")
	}
	if rows[0].LastLoginAt == nil {
		t.Error("first row has no last login, want the most recent sign-in first")
	}
	if rows[len(rows)-1].LastLoginAt != nil {
		t.Error("last row has a last login, want the never-signed-in account last")
	}
}

func TestSetUserSuperuserPromotesAndDemotes(t *testing.T) {
	user := seedRosterUser(t, rosterPrefix+"promote@example.com", true, false, nil)

	promoted, err := Connection().SetUserSuperuser(user.Email, true)
	if err != nil {
		t.Fatalf("SetUserSuperuser(true): %v", err)
	}
	if !promoted.Superuser {
		t.Error("Superuser = false after promotion, want true")
	}

	demoted, err := Connection().SetUserSuperuser(user.Email, false)
	if err != nil {
		t.Fatalf("SetUserSuperuser(false): %v", err)
	}
	if demoted.Superuser {
		t.Error("Superuser = true after demotion, want false")
	}
}

func TestSetUserSuperuserRejectsAnUnknownEmail(t *testing.T) {
	if _, err := Connection().SetUserSuperuser("no-such-account@example.com", true); err == nil {
		t.Error("error = nil, want an error for an unknown email")
	}
}
