package browser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// Every browser launch creates a chrome profile directory under
// launcher.DefaultUserDataDirPrefix. On hosts where the temp dir is a tmpfs those
// directories are held in RAM, so a pool that never removes them grows the scanner's
// footprint across every scan the process runs.
//
// These tests redirect the profile root to a per-test directory so they are unaffected
// by other sukyan processes running on the same host.

func isolateProfileRoot(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("ROD_USER_DATA_DIR"); dir != "" {
		t.Skip("ROD_USER_DATA_DIR pins the profile dir; cannot isolate")
	}
	root := filepath.Join(t.TempDir(), "user-data")
	previous := launcher.DefaultUserDataDirPrefix
	launcher.DefaultUserDataDirPrefix = root
	t.Cleanup(func() { launcher.DefaultUserDataDirPrefix = previous })
	return root
}

func profileDirs(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading profile root: %v", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	return out
}

func TestBrowserPoolCleanupRemovesProfileDirectories(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a browser")
	}
	root := isolateProfileRoot(t)

	manager := NewBrowserPoolManager(BrowserPoolManagerConfig{PoolSize: 2, Source: "test"}, 0, 0)

	var browsers []*rod.Browser
	for i := 0; i < 2; i++ {
		b, err := manager.NewBrowser()
		if err != nil {
			t.Skipf("cannot launch browser: %v", err)
		}
		browsers = append(browsers, b)
	}
	for _, b := range browsers {
		manager.ReleaseBrowser(b)
	}

	if got := len(profileDirs(t, root)); got != 2 {
		t.Fatalf("expected 2 profile directories while the pool is live, got %d", got)
	}

	manager.Cleanup()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(profileDirs(t, root)) == 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Errorf("pool Cleanup left %d profile directories behind: %v",
		len(profileDirs(t, root)), profileDirs(t, root))
}

func TestPagePoolManagerCloseRemovesProfileDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a browser")
	}
	root := isolateProfileRoot(t)

	manager, err := NewPagePoolManager(PagePoolManagerConfig{PoolSize: 2}, "test")
	if err != nil {
		t.Skipf("cannot launch browser: %v", err)
	}
	if got := len(profileDirs(t, root)); got != 1 {
		t.Fatalf("expected 1 profile directory while the page pool is live, got %d", got)
	}

	manager.Close()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if len(profileDirs(t, root)) == 0 {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Errorf("page pool Close left %d profile directories behind", len(profileDirs(t, root)))
}

// reapedPID returns the pid of a process that has already exited, so it is a pid no
// live browser can own.
func reapedPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("/bin/true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting probe process: %v", err)
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}

func writeFakeProfile(t *testing.T, root, name string, ownerPID int, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("creating fake profile: %v", err)
	}
	if ownerPID > 0 {
		host, _ := os.Hostname()
		target := fmt.Sprintf("%s-%d", host, ownerPID)
		if err := os.Symlink(target, filepath.Join(dir, "SingletonLock")); err != nil {
			t.Fatalf("creating SingletonLock: %v", err)
		}
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(dir, when, when); err != nil {
		t.Fatalf("setting mtime: %v", err)
	}
	return dir
}

func TestSweepStaleBrowserProfiles(t *testing.T) {
	root := isolateProfileRoot(t)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}

	stale := writeFakeProfile(t, root, "stale-dead-owner", reapedPID(t), 2*time.Hour)
	noOwner := writeFakeProfile(t, root, "stale-no-owner", 0, 2*time.Hour)
	recent := writeFakeProfile(t, root, "recent-no-owner", 0, time.Minute)
	live := writeFakeProfile(t, root, "live-owner", os.Getpid(), 2*time.Hour)

	removed, _ := SweepStaleBrowserProfiles(30 * time.Minute)

	for _, dir := range []string{stale, noOwner} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("expected %s to be swept, but it still exists", filepath.Base(dir))
		}
	}
	for _, dir := range []string{recent, live} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("expected %s to be preserved, but: %v", filepath.Base(dir), err)
		}
	}
	if removed != 2 {
		t.Errorf("expected 2 directories removed, got %d", removed)
	}
}

// A profile locked by a host other than this one must never be swept: on a shared
// volume we cannot tell whether the owning process is alive.
func TestSweepStaleBrowserProfilesSkipsForeignHostLocks(t *testing.T) {
	root := isolateProfileRoot(t)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}

	dir := filepath.Join(root, "foreign-host")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink("some-other-host-4242", filepath.Join(dir, "SingletonLock")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	when := time.Now().Add(-4 * time.Hour)
	if err := os.Chtimes(dir, when, when); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if removed, _ := SweepStaleBrowserProfiles(30 * time.Minute); removed != 0 {
		t.Errorf("expected foreign-host lock to be preserved, but %d were removed", removed)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("foreign-host profile was removed: %v", err)
	}
}
