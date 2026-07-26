package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// Browser-driven audits take a browser from a pool that lives for the whole scan, so
// anything left attached to it accumulates until the process exits. An incognito
// browser context holding a page keeps a renderer process alive; disposing the context
// (rod maps Browser.Close to Target.disposeBrowserContext when BrowserContextID is set)
// reclaims both.
//
// This guards the lifecycle contract that pkg/active/cspp.go and pkg/active/dom_xss.go
// rely on.

func countPageTargets(t *testing.T, b *rod.Browser) int {
	t.Helper()
	res, err := proto.TargetGetTargets{}.Call(b)
	if err != nil {
		t.Fatalf("TargetGetTargets: %v", err)
	}
	n := 0
	for _, ti := range res.TargetInfos {
		if ti.Type == "page" {
			n++
		}
	}
	return n
}

func countRenderers(t *testing.T, browserPID int) int {
	t.Helper()
	n := 0
	for _, pid := range childProcesses(browserPID) {
		raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
		if err != nil {
			continue
		}
		if strings.Contains(strings.ReplaceAll(string(raw), "\x00", " "), "--type=renderer") {
			n++
		}
	}
	return n
}

func childProcesses(root int) []int {
	parents := map[int]int{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", e.Name(), "stat"))
		if err != nil {
			continue
		}
		end := strings.LastIndex(string(stat), ")")
		if end < 0 {
			continue
		}
		fields := strings.Fields(string(stat)[end+1:])
		if len(fields) < 2 {
			continue
		}
		if ppid, err := strconv.Atoi(fields[1]); err == nil {
			parents[pid] = ppid
		}
	}
	var out []int
	for pid := range parents {
		for cur, depth := pid, 0; depth < 64; depth++ {
			parent, ok := parents[cur]
			if !ok || parent <= 1 {
				break
			}
			if parent == root {
				out = append(out, pid)
				break
			}
			cur = parent
		}
	}
	return out
}

func TestIncognitoContextReleasesPagesAndRenderers(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a browser")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h1>ok</h1></body></html>`)
	}))
	defer srv.Close()

	l := GetBrowserLauncher().Headless(true)
	controlURL, err := l.Launch()
	if err != nil {
		t.Skipf("cannot launch browser: %v", err)
	}
	b := rod.New().ControlURL(controlURL).MustConnect()
	defer func() {
		_ = b.Close()
		l.Kill()
	}()
	browserPID := l.PID()

	time.Sleep(500 * time.Millisecond)
	baseTargets, baseRenderers := countPageTargets(t, b), countRenderers(t, browserPID)

	const iterations = 8
	for i := 0; i < iterations; i++ {
		func() {
			incognito, err := b.Incognito()
			if err != nil {
				t.Fatalf("incognito: %v", err)
			}
			defer incognito.Close()

			page, err := incognito.Page(proto.TargetCreateTarget{URL: ""})
			if err != nil {
				t.Fatalf("page: %v", err)
			}
			if err := page.Navigate(srv.URL); err != nil {
				t.Fatalf("navigate: %v", err)
			}
			_ = page.WaitLoad()
		}()
	}

	time.Sleep(3 * time.Second)
	targets, renderers := countPageTargets(t, b), countRenderers(t, browserPID)

	if targets != baseTargets {
		t.Errorf("page targets leaked across %d incognito audits: baseline %d, now %d", iterations, baseTargets, targets)
	}
	if renderers != baseRenderers {
		t.Errorf("renderer processes leaked across %d incognito audits: baseline %d, now %d", iterations, baseRenderers, renderers)
	}
}
