package storyactions

import (
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/products"
)

// HandleSiteMap renders a site map of top products
func HandleSiteMap(w http.ResponseWriter, r *http.Request) error {

	// Build a query
	q := products.Query().Limit(5000)

	// Select only above 0 points,  Order by points, then id
	q.Where("points > 0").Order("points desc, id desc")

	// Fetch the products
	results, err := products.FindAll(q)
	if err != nil {
		return server.InternalError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.Layout("")
	view.Template("products/views/sitemap.xml.got")
	view.AddKey("products", results)
	view.AddKey("pubdate", productsModTimes(results))
	return view.Render()
}
