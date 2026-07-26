package testsupport

import (
	"context"
	"fmt"
	"sync"

	"github.com/DGosal/mini-erp/backend/internal/auth"
)

// FakeUsers is an in-memory auth.UserManager. Provisioning tests must not
// create real Firebase accounts: the suite would depend on the network, on a
// live project, and on cleanup that runs even when a test fails (§12.4).
//
// It records what it was asked to do, so a test can assert on the compensating
// delete of §3.3 step 4 — the part that is invisible in the response body and
// the part that leaves an orphaned account behind when it is missing.
type FakeUsers struct {
	mu      sync.Mutex
	byEmail map[string]string // email → uid, so duplicates behave like Firebase

	// ForceUID, if set, is the UID the next CreateUser returns, and is cleared
	// once used. For the §3.3-step-4 test, which needs the provider to hand back
	// a UID the database will refuse.
	ForceUID string

	// Created holds one entry per successful CreateUser, in order.
	Created []FakeAccount
	// Deleted holds every UID passed to DeleteUser, in order.
	Deleted []string
	// Disabled is the last state SetDisabled was called with, per UID.
	Disabled map[string]bool

	// FailCreate, if set, is returned by CreateUser instead of provisioning.
	// For the "provider is down" path.
	FailCreate error
}

// FakeAccount is one provisioned account.
type FakeAccount struct {
	UID         string
	Email       string
	Password    string
	DisplayName string
}

var _ auth.UserManager = (*FakeUsers)(nil)

func NewFakeUsers() *FakeUsers {
	return &FakeUsers{byEmail: map[string]string{}, Disabled: map[string]bool{}}
}

func (f *FakeUsers) CreateUser(_ context.Context, email, password, displayName string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.FailCreate != nil {
		return "", f.FailCreate
	}
	if _, exists := f.byEmail[email]; exists {
		return "", fmt.Errorf("%w: %s", auth.ErrEmailExists, email)
	}

	// Drawn from the package-wide counter, not a per-instance one: the database
	// is shared across every test in the process, and `users.firebase_uid` is
	// globally unique, so per-harness numbering would collide between tests.
	uid := fmt.Sprintf("fake-uid-%d", next())
	if f.ForceUID != "" {
		uid, f.ForceUID = f.ForceUID, ""
	}
	f.byEmail[email] = uid
	f.Created = append(f.Created, FakeAccount{
		UID: uid, Email: email, Password: password, DisplayName: displayName,
	})
	return uid, nil
}

func (f *FakeUsers) DeleteUser(_ context.Context, uid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Deleted = append(f.Deleted, uid)
	for email, u := range f.byEmail {
		if u == uid {
			delete(f.byEmail, email)
		}
	}
	return nil
}

func (f *FakeUsers) SetDisabled(_ context.Context, uid string, disabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.Disabled[uid] = disabled
	return nil
}

// WasDeleted reports whether uid was passed to DeleteUser.
func (f *FakeUsers) WasDeleted(uid string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, deleted := range f.Deleted {
		if deleted == uid {
			return true
		}
	}
	return false
}

// LastCreated returns the most recently provisioned account, and whether there
// was one.
func (f *FakeUsers) LastCreated() (FakeAccount, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if len(f.Created) == 0 {
		return FakeAccount{}, false
	}
	return f.Created[len(f.Created)-1], true
}
