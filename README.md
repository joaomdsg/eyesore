# eyesore

> **Research preview** — expect sharp edges and breaking changes.

Point at the ugly parts. An annotation overlay for your running web app:
click any element, leave a note, hit **Dispatch** — notes land as structured
JSON (selector, label, URL, element screenshot) ready to hand to a coding
agent.

## Usage

```sh
go run ./cmd/eyesore -url http://localhost:3000
```

Chromium opens on your app with the overlay injected. Toggle it with the
bottom-right switch, click elements to annotate, **Dispatch** writes
`eyesore-out/notes.json` + `eyesore-out/screenshots/*.png`.

Flags: `-url` app to annotate · `-out` notes output path · `-chrome` browser
binary (auto-detected by default).

## Roadmap

MCP server (agents pull notes, reply, mark fixed) · proxy-mode injection (no
managed browser) · deeper context capture (component source, console errors)
· agent self-verification with before/after screenshots.

MIT.
