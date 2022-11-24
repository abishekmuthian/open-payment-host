package storyactions

import (
	"net/http"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

// HandleListCode displays a list of products linking to repos (github etc)
// responds to GET /products/code
func HandleListCode(w http.ResponseWriter, r *http.Request) error {

	// Get params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Build a query
	q := products.Query().Where("points > -6").Order("rank desc, points desc, id desc").Limit(listLimit)

	// Restrict to products with have a url starting with github.com/abishekmuthian/open-payment-host or bitbucket.org
	// other code repos can be added later
	q.Where("url ILIKE 'https://github.com/abishekmuthian/open-payment-host%'").OrWhere("url ILIKE 'https://bitbucket.org%'")

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
	view.AddKey("page", page)
	view.AddKey("products", results)
	view.AddKey("pubdate", productsModTimes(results))
	view.AddKey("meta_title", "Go Code")
	view.AddKey("meta_desc", config.Get("meta_desc"))
	view.AddKey("meta_keywords", config.Get("meta_keywords"))
	view.AddKey("meta_rss", productsXMLPath(w, r))
	view.Template("products/views/index.html.got")
	view.AddKey("currentUser", session.CurrentUser(w, r))

	// If xml requested, serve with that template
	if strings.HasSuffix(r.URL.Path, ".xml") {
		view.Layout("")
		view.Template("products/views/index.xml.got")
	}

	return view.Render()

}
