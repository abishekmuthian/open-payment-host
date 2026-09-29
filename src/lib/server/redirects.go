package server

import (
	"fmt"
	"net/http"
	"strings"
)

// Redirect uses status 302 StatusFound by default - this is not a permanent redirect
// We don't accept external or relative paths for security reasons
func Redirect(w http.ResponseWriter, r *http.Request, path string) error {
	// 301 - http.StatusMovedPermanently - permanent redirect
	// 302 - http.StatusFound - tmp redirect
	return RedirectStatus(w, r, path, http.StatusFound)
}

// RedirectStatus redirects setting the status code (for example unauthorized)
// We don't accept external or relative paths for security reasons
func RedirectStatus(w http.ResponseWriter, r *http.Request, path string, status int) error {

	// We check this is an internal path - to redirect externally use http.Redirect directly
	if IsLocalPath(path) {
		// Status may be any value, e.g.
		// 301 - http.StatusMovedPermanently - permanent redirect
		// 302 - http.StatusFound - tmp redirect
		// 401 - Access denied
		http.Redirect(w, r, path, status)
		return nil
	}

	return fmt.Errorf("server: ignoring insecure redirect to external path %s", path)
}

// RedirectExternal redirects setting the status code
// (for example unauthorized), but does no checks on the path
// Use with caution and only on paths *fixed at compile time*.
func RedirectExternal(w http.ResponseWriter, r *http.Request, path string) error {
	http.Redirect(w, r, path, http.StatusFound)
	return nil
}

// IsLocalPath reports whether path is a same-origin absolute path. It rejects
// schemes, control characters (browsers strip tabs and newlines) and
// protocol-relative forms such as //host and /\host, which browsers treat as
// external URLs.
func IsLocalPath(path string) bool {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, ":") {
		return false
	}
	for _, c := range path {
		if c < 0x20 || c == 0x7f {
			return false
		}
	}
	return len(path) == 1 || (path[1] != '/' && path[1] != '\\')
}
