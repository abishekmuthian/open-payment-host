package appactions

import (
	"net/http"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/stats"
)

// HandleHome displays a list of products using gravity to order them
// used for the home page for gravity rank see votes.go
// responds to GET /
func HandleHome(w http.ResponseWriter, r *http.Request) error {
	stats.RegisterHit(r)

	currentUser := session.CurrentUser(w, r)

	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("home", 1)
	// view.AddKey("page", page)
	// view.AddKey("products", results)
	view.Template("app/views/home.html.got")
	// view.AddKey("pubdate", productsModTimes(results))
	view.AddKey("meta_url", config.Get("meta_url"))
	view.AddKey("meta_image", config.Get("meta_image"))
	view.AddKey("meta_foot", config.Get("meta_desc"))
	view.AddKey("meta_title", config.Get("meta_title"))
	view.AddKey("meta_desc", config.Get("meta_desc"))
	view.AddKey("meta_keywords", config.Get("meta_keywords"))
	// view.AddKey("meta_rss", productsXMLPath(w, r))
	view.AddKey("error", params.Get("error"))
	view.AddKey("notice", params.Get("notice"))
	view.AddKey("userCount", stats.UserCount())
	view.AddKey("currentUser", currentUser)

	// Add home key to set the CF turnstile script
	view.AddKey("home", true)
	// Set Cloudflare turnstile site key
	view.AddKey("turnstile_site_key", config.Get("turnstile_site_key"))

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	// For rss feeds use xml templates
	if strings.HasSuffix(r.URL.Path, ".xml") {
		view.Layout("")
		view.Template("products/views/index.xml.got")
	}

	return view.Render()

}
