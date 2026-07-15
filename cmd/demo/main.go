// Command demo serves the isore e2e fixture app (internal/demoapp) as a
// plain dev server, for annotating manually via `isore mcp` + start_proxy
// or for pointing integration tests at a real page instead of an inline
// HTML literal.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/joaomdsg/isore/internal/demoapp"
)

func main() {
	port := flag.Int("port", 3000, "port to serve the demo app on")
	flag.Parse()

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	fmt.Printf("isore demo app serving http://%s\n", addr)
	if err := http.ListenAndServe(addr, http.FileServer(http.FS(demoapp.FS()))); err != nil {
		fmt.Fprintln(os.Stderr, "isore demo:", err)
		os.Exit(1)
	}
}
