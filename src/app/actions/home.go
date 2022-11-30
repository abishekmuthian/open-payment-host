package appactions

import (
	"net/http"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"
	"github.com/abishekmuthian/open-payment-host/src/products"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/stats"
)

// HandleHome displays a list of products using gravity to order them
// used for the home page for gravity rank see votes.go
// responds to GET /
func HandleHome(w http.ResponseWriter, r *http.Request) error {

	// FIXME listLimit should be int64 to reflect page, so needs changes in query limit
	const listLimit = 9

	// Build a query
	q := products.Query().Limit(listLimit)

	// Select only above 0 points and status is null or not suspended or in draft,  Order by rank, then points, then name
	q.Where("points > 0").Order("points desc, points desc")

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	productsCount, _ := products.Query().Count()

	// Set the offset in pages if we have one
	page := int(params.GetInt("page"))
	if page > 0 {
		q.Offset(listLimit * page)
	}

	// Fetch the products
	results, err := products.FindAll(q)
	if err != nil {
		return server.InternalError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("home", 1)
	view.AddKey("page", page)
	view.AddKey("productsCount", productsCount)
	view.AddKey("products", results)
	view.Template("products/views/home.html.got")
	view.AddKey("meta_url", config.Get("meta_url"))
	view.AddKey("meta_image", config.Get("meta_image"))
	view.AddKey("meta_foot", config.Get("meta_desc"))
	view.AddKey("meta_title", config.Get("meta_title"))
	view.AddKey("meta_desc", config.Get("meta_desc"))
	view.AddKey("meta_keywords", config.Get("meta_keywords"))
	view.AddKey("userCount", stats.UserCount())
	view.AddKey("currentUser", session.CurrentUser(w, r))

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
