package storyactions

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/stats"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

// FIXME listLimit should be int64 to reflect page, so needs changes in query limit
const listLimit = 50

// HandleIndex repsponds to GET /stories
func HandleIndex(w http.ResponseWriter, r *http.Request) error {

	// No Authorisation - anyone can view stories

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	stats.RegisterHit(r)

	// Build a query
	q := products.Query().Limit(listLimit)

	// Get current user
	currentUser := session.CurrentUser(w, r)

	if currentUser.Admin() {
		// Order by date by default and show all products for Admin
		q.Order("created_at desc")
	} else {
		// Order by date by default and status is null or status not suspended or in draft
		q.Where("id > 5")
		q.Where("points > 0 AND status IS NULL OR status NOT IN (50,1)").Order("created_at desc")
	}

	// Filter if necessary - this assumes name and summary cols
	filter := params.Get("q")
	if len(filter) > 0 {

		// Replace special characters with escaped sequence
		filter = strings.Replace(filter, "_", "\\_", -1)
		filter = strings.Replace(filter, "%", "\\%", -1)

		wildcard := "%" + filter + "%"

		// Perform a wildcard search for name or url
		q.Where("products.name ILIKE ? OR products.summary ILIKE ?", wildcard, wildcard)

		// If filtering, order by rank, not by date
		q.Order("rank desc, points desc, id desc")
	}

	// Set the offset in pages if we have one
	page := int(params.GetInt("page")) //converting int64 to int
	if page > 0 {
		q.Offset(listLimit * int(page))
	}

	// Fetch the products
	results, err := products.FindAll(q)
	if err != nil {
		return server.InternalError(err)
	}

	windowTitle := config.Get("meta_title")
	switch filter {
	case "Video:":
		windowTitle = "open-payment-host Videos"
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("index", 1)
	view.AddKey("page", page)
	view.AddKey("products", results)
	view.AddKey("pubdate", productsModTimes(results))
	view.AddKey("meta_url", config.Get("meta_url"))
	view.AddKey("meta_title", windowTitle)
	view.AddKey("meta_desc", config.Get("meta_desc"))
	view.AddKey("meta_image", config.Get("meta_image"))
	view.AddKey("meta_keywords", config.Get("meta_keywords"))
	view.AddKey("meta_foot", config.Get("meta_desc"))
	view.AddKey("meta_rss", productsXMLPath(w, r))
	view.AddKey("currentUser", currentUser)
	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	if strings.HasSuffix(r.URL.Path, ".xml") {
		view.Layout("")
		view.Template("products/views/index.xml.got")
	}

	return view.Render()

}

// storiesModTime returns the mod time of the first story, or current time if no stories
func productsModTimes(availableStories []*products.Story) time.Time {
	if len(availableStories) == 0 {
		return time.Now()
	}
	story := availableStories[0]

	return story.UpdatedAt
}

// storiesXMLPath returns the xml path for a given request to a stories link
func productsXMLPath(w http.ResponseWriter, r *http.Request) string {

	p := strings.Replace(r.URL.Path, ".xml", "", 1)
	if p == "/" {
		p = "/index"
	}

	q := r.URL.RawQuery
	if len(q) > 0 {
		q = "?" + q
	}

	return fmt.Sprintf("%s.xml%s", p, q)
}
