package app

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
)

// Serve static files (assets, images etc)
func fileHandler(w http.ResponseWriter, r *http.Request) error {

	// First try serving assets
	err := serveAsset(w, r)
	if err == nil {
		return nil
	}

	// If assets fail, try to serve file in public
	return serveFile(w, r)
}

// serveFile serves a file from ./public if it exists
func serveFile(w http.ResponseWriter, r *http.Request) error {

	// Try a local path in the public directory
	localPath := "./public" + path.Clean(r.URL.Path)
	s, err := os.Stat(localPath)
	if err != nil {
		// If file not found return 404
		if os.IsNotExist(err) {
			return server.NotFoundError(err)
		}

		// For other errors return not authorised
		return server.NotAuthorizedError(err)
	}

	// If not a file return immediately
	if s.IsDir() {
		return nil
	}

	// If the file exists and we can access it, serve it with cache control
	if config.Production() {
		addCacheControl(w, r)
	}

	http.ServeFile(w, r, localPath)
	return nil
}

// serveAsset serves a file from ./public/assets usings appAssets
func serveAsset(w http.ResponseWriter, r *http.Request) error {

	p := path.Clean(r.URL.Path)

	// It must be under /assets, or we don't serve
	if !strings.HasPrefix(p, "/assets/") {
		return server.NotFoundError(nil)
	}

	// Try to find an asset in our list
	f := appAssets.File(path.Base(p))
	if f == nil {
		return server.NotFoundError(nil)
	}

	// Serve the local file, with cache control
	localPath := "./" + f.LocalPath()
	// If the file exists and we can access it, serve it with cache control in production
	if config.Production() {
		addCacheControl(w, r)
	}
	http.ServeFile(w, r, localPath)
	return nil
}

// errHandler renders an error using error templates if available
func errHandler(w http.ResponseWriter, r *http.Request, e error) {

	// Cast the error to a status error if it is one, if not wrap it in a Status 500 error
	err := server.ToStatusError(e)
	log.Error(log.V{"error": err})

	view := view.NewWithPath("", w)
	view.AddKey("title", err.Title)
	view.AddKey("message", err.Message)
	// In production, provide no detail for security reasons
	if !config.Production() {
		view.AddKey("status", err.Status)
		view.AddKey("file", err.FileLine())
		view.AddKey("error", err.Err)
	}
	view.AddKey("name", config.Get("name"))
	view.Template("app/views/error.html.got")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(err.Status)
	view.Render()
}

// hash returns the sha hash of a string
func hash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString([]byte(sum[:]))
}

func addCacheControl(w http.ResponseWriter, r *http.Request) {
	// Cache for 30 days
	w.Header().Set("Cache-Control", "max-age:2592000")

	// Set an expires header Mon Jan 2 15:04:05 -0700 MST 2006
	w.Header().Set("Expires", time.Now().AddDate(0, 0, 30).UTC().Format("Mon, 2 Jan 2006 15:04:05 MST"))

	// For etag Just hash the path - static resources are assumed to have a fingerprint
	w.Header().Set("ETag", fmt.Sprintf("\"%s\"", hash(r.URL.Path)))
}
