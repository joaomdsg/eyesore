package main

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/joaomdsg/eyesore/internal/store"
	"github.com/stretchr/testify/require"
)

func htmlBackend(t *testing.T) *url.URL {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<html><body>hi</body></html>")
	}))
	t.Cleanup(s.Close)
	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	return u
}

func getBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

func freshStore(t *testing.T) *store.Store {
	t.Helper()
	return store.New(filepath.Join(t.TempDir(), "notes.json"))
}

func TestStartProxyServesInjectedApp(t *testing.T) {
	h := &proxyHolder{}
	base, err := h.start(htmlBackend(t), "127.0.0.1:0", freshStore(t), []byte(overlayJS), 20*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.http.Close() })

	body := getBody(t, base+"/")
	require.Contains(t, body, `<script src="/__eyesore/overlay.js"></script>`)
	require.Contains(t, body, "hi")
}

func TestStartProxyReInvokeReplacesPrevious(t *testing.T) {
	target := htmlBackend(t)
	h := &proxyHolder{}

	base1, err := h.start(target, "127.0.0.1:0", freshStore(t), []byte(overlayJS), 20*time.Millisecond)
	require.NoError(t, err)
	base2, err := h.start(target, "127.0.0.1:0", freshStore(t), []byte(overlayJS), 20*time.Millisecond)
	require.NoError(t, err)
	require.NotEqual(t, base1, base2)
	t.Cleanup(func() { _ = h.http.Close() })

	require.Contains(t, getBody(t, base2+"/"), "hi")

	// The previous listener was torn down synchronously; it must refuse now.
	if _, err := http.Get(base1 + "/"); err == nil {
		t.Fatalf("old proxy at %s still serving after restart", base1)
	}
}

func TestStartProxyBindErrorSurfaces(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = occupied.Close() })

	target, _ := url.Parse("http://127.0.0.1:3000")
	h := &proxyHolder{}
	_, err = h.start(target, occupied.Addr().String(), freshStore(t), []byte(overlayJS), 20*time.Millisecond)
	require.Error(t, err)
	require.Nil(t, h.srv, "no proxy should be swapped in on bind failure")
}

func TestProxyHolderReloadGating(t *testing.T) {
	h := &proxyHolder{}
	require.False(t, h.reload(), "reload before start must report no proxy")

	_, err := h.start(htmlBackend(t), "127.0.0.1:0", freshStore(t), []byte(overlayJS), 20*time.Millisecond)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.http.Close() })

	require.True(t, h.reload(), "reload after start must drive the in-process proxy")
}
