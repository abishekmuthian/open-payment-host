package storyactions

import (
	"net/http"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/lib/stats"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

// HandleListUpvoted displays a list of products the user has upvoted in the past
func HandleListUpvoted(w http.ResponseWriter, r *http.Request) error {
	stats.RegisterHit(r)

	// Build a query
	q := products.Query().Limit(listLimit)

	// Select only above 0 points,  Order by rank, then points, then name
	q.Where("points > 0").Order("rank desc, points desc, id desc")

	// Select only products which the user has upvoted
	user := session.CurrentUser(w, r)
	if !user.Anon() {
		// Can we use a join instead?
		v := query.New("votes", "story_id").Select("select story_id as id from votes").Where("user_id=? AND story_id IS NOT NULL AND points > 0", user.ID)

		storyIDs := v.ResultIDs()
		if len(storyIDs) > 0 {
			q.WhereIn("id", storyIDs)
		}
	}

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

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
	view.AddKey("upvoted", 1)
	view.AddKey("meta_url", config.Get("meta_url"))
	view.AddKey("meta_image", config.Get("meta_image"))
	view.AddKey("meta_foot", config.Get("meta_desc"))
	view.AddKey("meta_title", "products you have upvoted")
	view.AddKey("meta_desc", config.Get("meta_desc"))
	view.AddKey("meta_keywords", config.Get("meta_keywords"))
	view.AddKey("meta_rss", productsXMLPath(w, r))
	view.Template("products/views/index.html.got")
	view.AddKey("currentUser", session.CurrentUser(w, r))

	if strings.HasSuffix(r.URL.Path, ".xml") {
		view.Layout("")
		view.Template("products/views/index.xml.got")
	}

	return view.Render()

}
