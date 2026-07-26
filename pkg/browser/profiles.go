package browser

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/rs/zerolog/log"
)

const (
	// How long a browser is given to exit before its profile directory is removed
	// anyway. rod's Launcher.Cleanup blocks until the process exits, which never
	// happens if the browser is wedged.
	browserExitTimeout = 15 * time.Second

	// Profiles younger than this are never swept, so a browser that is still starting
	// up and has not yet written its lock file is left alone.
	staleProfileMinAge = 30 * time.Minute
)

var profileSweepOnce sync.Once

// discardLauncher removes the profile directory belonging to a launched browser. It
// bounds rod's Launcher.Cleanup, which waits on the browser process forever.
func discardLauncher(l *launcher.Launcher) {
	if l == nil {
		return
	}
	dir := l.Get(flags.UserDataDir)

	done := make(chan struct{})
	go func() {
		l.Cleanup()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(browserExitTimeout):
		log.Warn().Str("dir", dir).Msg("Browser did not exit in time, killing it to release its profile directory")
		l.Kill()
		if dir != "" {
			if err := os.RemoveAll(dir); err != nil {
				log.Warn().Err(err).Str("dir", dir).Msg("Failed to remove browser profile directory")
			}
		}
	}
}

// sweepStaleProfilesOnce reclaims profile directories orphaned by earlier runs. Scans
// are routinely cancelled or killed, so the pool's own cleanup can never be the only
// thing that removes them.
func sweepStaleProfilesOnce() {
	profileSweepOnce.Do(func() {
		removed, reclaimed := SweepStaleBrowserProfiles(staleProfileMinAge)
		if removed > 0 {
			log.Info().
				Int("directories", removed).
				Int64("bytes", reclaimed).
				Msg("Removed stale browser profile directories")
		}
	})
}

// SweepStaleBrowserProfiles removes rod user-data directories left behind by browsers
// that are no longer running. A directory is preserved if a live browser still owns it
// or if it was modified within minAge, so a concurrently running scan is never
// disturbed. It returns how many directories were removed and how many bytes that
// reclaimed.
func SweepStaleBrowserProfiles(minAge time.Duration) (removed int, reclaimed int64) {
	root := launcher.DefaultUserDataDirPrefix
	entries, err := os.ReadDir(root)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Debug().Err(err).Str("root", root).Msg("Could not read browser profile root")
		}
		return 0, 0
	}

	cutoff := time.Now().Add(-minAge)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())

		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if profileHasLiveOwner(dir) {
			continue
		}

		size := directorySize(dir)
		if err := os.RemoveAll(dir); err != nil {
			log.Debug().Err(err).Str("dir", dir).Msg("Could not remove stale browser profile")
			continue
		}
		removed++
		reclaimed += size
	}
	return removed, reclaimed
}

// profileHasLiveOwner reports whether a browser still owns a profile directory. Chrome
// writes SingletonLock as a symlink to "<hostname>-<pid>" and leaves it in place after
// an unclean exit, so the link identifies the owner but does not prove it is alive.
func profileHasLiveOwner(dir string) bool {
	target, err := os.Readlink(filepath.Join(dir, "SingletonLock"))
	if err != nil {
		return false
	}

	separator := strings.LastIndex(target, "-")
	if separator < 0 {
		return true
	}

	host, err := os.Hostname()
	if err != nil || target[:separator] != host {
		// Written by another machine on a shared volume; its liveness is unknowable.
		return true
	}

	pid, err := strconv.Atoi(target[separator+1:])
	if err != nil || pid <= 0 {
		return true
	}
	return processExists(pid)
}

func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

func directorySize(dir string) (total int64) {
	_ = filepath.WalkDir(dir, func(_ string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
