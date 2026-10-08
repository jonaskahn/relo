// Package dashboard serves the compiled console: the static files the
// Svelte console builds, embedded into the binary at compile time.
package dashboard

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

//go:embed all:dist
var files embed.FS

const embeddedRoot = "dist"

const shellName = "index.html"

const headClose = "</head>"

const immutablePrefix = "_app/immutable/"

// Options configures the console handler.
type Options struct {
	// Language reports the language the console should speak, which the
	// handler hands the page instead of making the browser ask for it.
	// A nil function serves the shell exactly as the build produced it.
	Language   func() string
	Appearance func() (theme, accent string)
}

// New returns the console over the given file tree. A tree without the
// application shell yields a handler that reports the console was never
// built, which is what a binary compiled without a console build serves.
func New(content fs.FS, options Options) http.Handler {
	shell, err := fs.ReadFile(content, shellName)
	if err != nil {
		return http.HandlerFunc(unbuilt)
	}
	return &console{
		content: content, shell: shell, language: options.Language, appearance: options.Appearance,
		files: http.FileServerFS(content),
	}
}

// Handler returns the console this binary embedded.
func Handler(options Options) http.Handler {
	content, err := fs.Sub(files, embeddedRoot)
	if err != nil {
		return http.HandlerFunc(unbuilt)
	}
	return New(content, options)
}

// Built reports whether this binary embedded a compiled console.
func Built() bool {
	content, err := fs.Sub(files, embeddedRoot)
	if err != nil {
		return false
	}
	_, err = fs.Stat(content, shellName)
	return err == nil
}

type console struct {
	content    fs.FS
	shell      []byte
	language   func() string
	appearance func() (string, string)
	files      http.Handler
}

// ServeHTTP serves the embedded console: stored files by name and the shell
// for every other read-only path, refusing writes the console never makes.
func (c *console) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "the console is read-only", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name != "" && isFile(c.content, name) {
		w.Header().Set("Cache-Control", cachePolicy(name))
		c.files.ServeHTTP(w, r)
		return
	}
	if name != "" && hasExtension(name) {
		http.NotFound(w, r)
		return
	}
	c.serveShell(w, r)
}

func (c *console) serveShell(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(c.shellFor())
}

func (c *console) shellFor() []byte {
	shell := c.shell
	if c.language != nil {
		if tag := c.language(); tag != "" {
			shell = injectLanguage(shell, tag)
		}
	}
	if c.appearance != nil {
		theme, accent := c.appearance()
		shell = injectAppearance(shell, theme, accent)
	}
	return shell
}

func injectAppearance(shell []byte, theme, accent string) []byte {
	quotedTheme, quotedAccent := strconv.Quote(theme), strconv.Quote(accent)
	script := []byte("<script>window.__reloAppearance={theme:" + quotedTheme + ",accent:" + quotedAccent +
		"};document.documentElement.dataset.accent=" + quotedAccent +
		";if(" + quotedTheme + "==='dark'||(" + quotedTheme + "==='system'&&matchMedia('(prefers-color-scheme: dark)').matches))document.documentElement.classList.add('dark');</script>")
	at := bytes.Index(shell, []byte(headClose))
	if at < 0 {
		return append(append(make([]byte, 0, len(shell)+len(script)), shell...), script...)
	}
	injected := make([]byte, 0, len(shell)+len(script))
	injected = append(injected, shell[:at]...)
	injected = append(injected, script...)
	return append(injected, shell[at:]...)
}

func injectLanguage(shell []byte, tag string) []byte {
	quoted := strconv.Quote(tag)
	script := []byte("<script>window.__reloLanguage=" + quoted +
		";document.documentElement.lang=" + quoted + ";</script>")
	at := bytes.Index(shell, []byte(headClose))
	if at < 0 {
		return append(append(make([]byte, 0, len(shell)+len(script)), shell...), script...)
	}
	injected := make([]byte, 0, len(shell)+len(script))
	injected = append(injected, shell[:at]...)
	injected = append(injected, script...)
	return append(injected, shell[at:]...)
}

func isFile(content fs.FS, name string) bool {
	info, err := fs.Stat(content, name)
	return err == nil && !info.IsDir()
}

func hasExtension(name string) bool {
	return path.Ext(path.Base(name)) != ""
}

func cachePolicy(name string) string {
	if strings.HasPrefix(name, immutablePrefix) {
		return "public, max-age=31536000, immutable"
	}
	return "no-cache"
}

func unbuilt(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = io.WriteString(w, `{"error":"not_implemented","message":"this build has no dashboard"}`+"\n")
}
