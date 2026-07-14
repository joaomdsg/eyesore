package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/joaomdsg/eyesore/internal/proxy"
	"github.com/joaomdsg/eyesore/internal/serve"
	"github.com/joaomdsg/eyesore/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// openBrowser best-effort launches the user's default browser at rawURL.
// A package var so tests can stub it — the real one shells out per platform.
var openBrowser = func(rawURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	_ = cmd.Start()
}

// proxyHolder owns the at-most-one in-process reverse proxy started via
// start_proxy. Every field access is under mu so a restart (which tears down
// the previous proxy) races safely with reload.
type proxyHolder struct {
	mu   sync.Mutex
	srv  *proxy.Server
	http *http.Server
	base string
}

// start binds listen synchronously — so "address already in use" surfaces to
// the caller rather than dying in a goroutine — then tears down any previous
// proxy and serves the new one in the background. Returns the base URL actually
// bound (honoring :0 in tests).
func (h *proxyHolder) start(target *url.URL, listen string, st *store.Store, overlay []byte, poll time.Duration, opts ...proxy.Option) (string, error) {
	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return "", err
	}
	p := proxy.NewServer(target, st, overlay, poll, opts...)
	srv := &http.Server{Handler: p}
	base := "http://" + ln.Addr().String()

	h.mu.Lock()
	oldSrv, oldProxy := h.http, h.srv
	h.srv, h.http, h.base = p, srv, base
	h.mu.Unlock()

	if oldSrv != nil {
		_ = oldSrv.Close()
	}
	if oldProxy != nil {
		oldProxy.Close()
	}
	go srv.Serve(ln)
	return base, nil
}

// reload refreshes every connected tab of the in-process proxy. Returns false
// when no proxy has been started, letting the caller fall back to an
// externally-run proxy's reload endpoint.
func (h *proxyHolder) reload() bool {
	h.mu.Lock()
	p := h.srv
	h.mu.Unlock()
	if p == nil {
		return false
	}
	p.Reload()
	return true
}

type listIn struct{}

type notesOut struct {
	Notes []serve.NoteView `json:"notes"`
}

type awaitIn struct {
	SinceMs        int64 `json:"sinceMs,omitempty" jsonschema:"only notes dispatched after this unix-ms timestamp; 0 means anything dispatched from now on"`
	TimeoutSeconds int   `json:"timeoutSeconds,omitempty" jsonschema:"how long to wait before returning empty; default 120"`
}

type markFixedIn struct {
	ID      string `json:"id" jsonschema:"id of the note that has been fixed"`
	Summary string `json:"summary,omitempty" jsonschema:"one line for the user: what you changed and why"`
}

type markWorkingIn struct {
	ID string `json:"id" jsonschema:"id of the note you are starting on"`
}

type startProxyIn struct {
	TargetPort int `json:"targetPort,omitempty" jsonschema:"port your running dev server listens on and that eyesore will annotate; default 3000"`
	ProxyPort  int `json:"proxyPort,omitempty" jsonschema:"port to serve the annotated app on; default 4400"`
}

type startProxyOut struct {
	URL     string `json:"url"`
	Message string `json:"message"`
}

type emptyIn struct{}

type okOut struct {
	OK bool `json:"ok"`
}

type markFixedOut struct {
	Fixed string `json:"fixed"`
}

func runMCP(args []string) error {
	fs := flag.NewFlagSet("eyesore mcp", flag.ExitOnError)
	out := fs.String("out", "eyesore-out/notes.json", "notes store shared with the harness")
	if err := fs.Parse(args); err != nil {
		return err
	}

	h := serve.New(store.New(*out), filepath.Dir(*out), func() int64 {
		return time.Now().UnixMilli()
	})

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "eyesore",
		Title:   "Eyesore UI annotations",
		Version: "0.1.0",
	}, nil)

	holder := &proxyHolder{}
	// Dedicated pool + mutex so dispatch-time element captures never interleave
	// navigations with the browser_* tools' pool (one driver, one tab).
	shootPool := &driverPool{outDir: filepath.Dir(*out)}
	var shootMu sync.Mutex
	shoot := func(pageURL, selector string) ([]byte, error) {
		shootMu.Lock()
		defer shootMu.Unlock()
		d, err := shootPool.get(context.Background())
		if err != nil {
			return nil, err
		}
		if err := d.Navigate(pageURL); err != nil {
			return nil, err
		}
		return d.Screenshot(selector)
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_notes",
		Description: "List pending UI annotations the user dispatched from the eyesore overlay: what to change, on which element (CSS selector). ALWAYS call get_screenshot for each note to see what the user saw, then fix and call mark_fixed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ listIn) (*mcp.CallToolResult, notesOut, error) {
		ns, err := h.ListNotes(ctx)
		return nil, notesOut{Notes: ns}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "await_notes",
		Description: "Block until the user dispatches new annotations from the eyesore overlay, then return them. Empty result means the wait timed out — call again to keep listening.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in awaitIn) (*mcp.CallToolResult, notesOut, error) {
		timeout := 120 * time.Second
		if in.TimeoutSeconds > 0 {
			timeout = time.Duration(in.TimeoutSeconds) * time.Second
		}
		waitCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		ns, err := h.Await(waitCtx, in.SinceMs)
		return nil, notesOut{Notes: ns}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mark_fixed",
		Description: "Mark a note as fixed once the requested change is implemented, with a one-line summary. The user sees the badge turn green and reads your summary in the overlay.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in markFixedIn) (*mcp.CallToolResult, markFixedOut, error) {
		if err := h.MarkFixed(ctx, in.ID, in.Summary); err != nil {
			return nil, markFixedOut{}, err
		}
		return nil, markFixedOut{Fixed: in.ID}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mark_working",
		Description: "Flag a note as picked up BEFORE you start changing code — the user's overlay badge turns amber so they know you are on it.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in markWorkingIn) (*mcp.CallToolResult, okOut, error) {
		if err := h.MarkWorking(ctx, in.ID); err != nil {
			return nil, okOut{}, err
		}
		return nil, okOut{OK: true}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "start_proxy",
		Description: "Start eyesore in proxy mode: run a reverse proxy that injects the annotation overlay in front of the user's dev server, and open their browser to it. Pass the port your dev server listens on (targetPort) and the port to serve the annotated app on (proxyPort). Calling again restarts the proxy on the new ports. After it returns, call await_notes to receive the user's annotations.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in startProxyIn) (*mcp.CallToolResult, startProxyOut, error) {
		targetPort := in.TargetPort
		if targetPort == 0 {
			targetPort = 3000
		}
		proxyPort := in.ProxyPort
		if proxyPort == 0 {
			proxyPort = 4400
		}
		target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", targetPort))
		if err != nil {
			return nil, startProxyOut{}, err
		}
		listen := fmt.Sprintf("127.0.0.1:%d", proxyPort)
		base, err := holder.start(target, listen, store.New(*out), []byte(overlayJS), 300*time.Millisecond, proxy.WithShooter(shoot))
		if err != nil {
			return nil, startProxyOut{}, fmt.Errorf("start proxy on %s: %w", listen, err)
		}
		openBrowser(base)
		return nil, startProxyOut{
			URL:     base,
			Message: fmt.Sprintf("Proxy live: annotating http://127.0.0.1:%d at %s (browser opened). Ctrl-Shift-N toggles the overlay; call await_notes for dispatches.", targetPort, base),
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "reload_page",
		Description: "Refresh the user's browser tabs after you rebuilt the app. Call once your fix is live so the user sees it immediately. Requires start_proxy to have been called first.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ emptyIn) (*mcp.CallToolResult, okOut, error) {
		if !holder.reload() {
			return nil, okOut{}, fmt.Errorf("no proxy running — call start_proxy first")
		}
		return nil, okOut{OK: true}, nil
	})

	addBrowserTools(server, h, &driverPool{outDir: filepath.Dir(*out)})

	fmt.Fprintf(os.Stderr, "eyesore mcp: serving stdio, store %s\n", *out)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}
