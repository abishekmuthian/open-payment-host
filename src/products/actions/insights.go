package storyactions

import (
	"fmt"
	"github.com/abishekmuthian/open-payment-host/src/lib/analytics"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"net/http"
	"time"
)

// HandleInsights handles the POST from /products/{id:[0-9]+}/insights
func HandleInsights(w http.ResponseWriter, r *http.Request) error {

	return nil
}

// UpdateInsights updates the insights on the database
func UpdateInsights() {
	productCount, err := products.Query().Count()
	log.Info(log.V{"Insights": "Updating Insights", "Total product count": productCount})
	if err == nil {
		for ; productCount >= 1; productCount-- {
			// Get the insights
			log.Info(log.V{"Insights": "Updating Insights", "product ID": productCount})
			allTimeTop3Countries, allTimePageViews, _ := GetAllTimeStoryInsights(productCount)

			sevenDaysTop3Countries, sevenDaysPageViews, _ := GetSevenDaysStoryInsights(productCount)

			thirtyDaysTop3Countries, thirtyDaysPageViews, _ := GetThirtyDaysStoryInsights(productCount)

			err := updateStoryInsights(productCount, allTimePageViews, allTimeTop3Countries, sevenDaysPageViews, sevenDaysTop3Countries, thirtyDaysPageViews, thirtyDaysTop3Countries)
			if err != nil {
				log.Error(log.V{"Insights": "Updating Insights", "Error updating database": err})
			}
		}
	}
}

// GetAllTimeStoryInsights fetches all time story insights
func GetAllTimeStoryInsights(productID int64) (string, int64, error) {

	allTimeTop3Countries, allTimePageViews, err := analytics.AllTimeViews(productID)
	if err != nil {
		return "", 0, err
	}

	return allTimeTop3Countries, allTimePageViews, nil
}

// GetSevenDaysStoryInsights fetches seven days top3 countries
func GetSevenDaysStoryInsights(productID int64) (string, int64, error) {

	sevenDaysTop3Countries, sevenDaysPageViews, err := analytics.SevenDaysViews(productID)
	if err != nil {
		return "", 0, err
	}

	return sevenDaysTop3Countries, sevenDaysPageViews, nil
}

// GetThirtyDaysStoryInsights fetches thirty days top3 countries
func GetThirtyDaysStoryInsights(productID int64) (string, int64, error) {

	thirtyDaysTop3Countries, thirtyDaysPageViews, err := analytics.ThirtyDaysViews(productID)
	if err != nil {
		return "", 0, err
	}

	return thirtyDaysTop3Countries, thirtyDaysPageViews, nil
}

// updateStoryInsights updates a story for new comment counts
func updateStoryInsights(productID int64, allTimePageViews int64, allTimeTop3Countries string, sevenDaysPageViews int64, sevenDaysTop3Countries string, thirtyDaysPageViews int64, thirtyDaysTop3Countries string) error {
	story, err := products.Find(productID)
	if err == nil {
		storyParams := map[string]string{"all_time_page_views": fmt.Sprintf("%d", allTimePageViews), "all_time_top3_countries": fmt.Sprintf("%s", allTimeTop3Countries), "seven_days_page_views": fmt.Sprintf("%d", sevenDaysPageViews), "seven_days_top3_countries": fmt.Sprintf("%s", sevenDaysTop3Countries), "thirty_days_page_views": fmt.Sprintf("%d", thirtyDaysPageViews), "thirty_days_top3_countries": fmt.Sprintf("%s", thirtyDaysTop3Countries), "insights_updated": query.TimeString(time.Now().UTC())}
		return story.Update(storyParams)
	} else {
		return err
	}

}
