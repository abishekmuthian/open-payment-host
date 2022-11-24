package analytics

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	ga "google.golang.org/api/analyticsreporting/v4"
	"google.golang.org/api/option"
)

func AllTimeViews(productID int64) (top3Countries string, totalPageViews int64, err error) {
	svc, err := makeReportSvc()

	if err != nil {
		log.Error(log.V{"Google Analytics": "Failed to create Google Analytics Reporting Service"})
	}

	res, err := getReport(svc, config.Get("analytics_view_id"), config.Get("analytics_start_date"), productID)

	if err != nil {
		log.Error(log.V{"Google Analytics": err})
	}

	log.Info(log.V{"Google Analytics": "Getting all time insights."})

	top3Countries, totalPageViews, err = printResponse(res)

	if err != nil {
		log.Error(log.V{"Google Analytics": err})
	}

	return top3Countries, totalPageViews, err

}

func SevenDaysViews(productID int64) (top3Countries string, totalPageViews int64, err error) {
	svc, err := makeReportSvc()

	if err != nil {
		log.Error(log.V{"Google Analytics": "Failed to create Google Analytics Reporting Service"})
	}

	res, err := getReport(svc, config.Get("analytics_view_id"), "7daysAgo", productID)

	if err != nil {
		log.Error(log.V{"Google Analytics": err})
	}

	log.Info(log.V{"Google Analytics": "Getting Seven days insights."})

	top3Countries, totalPageViews, err = printResponse(res)

	if err != nil {
		log.Error(log.V{"Google Analytics": err})
	}

	return top3Countries, totalPageViews, err
}

func ThirtyDaysViews(productID int64) (top3Countries string, totalPageViews int64, err error) {
	svc, err := makeReportSvc()

	if err != nil {
		log.Error(log.V{"Google Analytics": "Failed to create Google Analytics Reporting Service"})
	}

	res, err := getReport(svc, config.Get("analytics_view_id"), "30daysAgo", productID)

	if err != nil {
		log.Error(log.V{"Google Analytics": err})
	}

	log.Info(log.V{"Google Analytics": "Getting 30 days insights."})

	top3Countries, totalPageViews, err = printResponse(res)

	if err != nil {
		log.Error(log.V{"Google Analytics": err})
	}

	return top3Countries, totalPageViews, err
}

// makeReportSvc initializes and returns an authorized
// Analytics Reporting API V4 service object.
func makeReportSvc() (*ga.Service, error) {

	ctx := context.Background()
	svc, err := ga.NewService(ctx, option.WithCredentialsFile(config.Get("analytics_credentials")))

	if err != nil {
		log.Error(log.V{"Google Analytics": err})
		return nil, err
	}

	log.Info(log.V{"Google Analytics": "Created Google Analytics Reporting Service object"})

	return svc, nil
}

// getReport queries the Analytics Reporting API V4 using
// the Analytics Reporting API V4 service object.
// It returns the Analytics Reporting API V4 response
func getReport(svc *ga.Service, viewId string, startDate string, productID int64) (*ga.GetReportsResponse, error) {
	//defer TimeTrack(time.Now(), "GET Analytics Report")
	// A GetReportsRequest instance is a batch request
	// which can have a maximum of 5 requests
	req := &ga.GetReportsRequest{
		// Our request contains only one request
		// So initialise the slice with one ga.ReportRequest object
		ReportRequests: []*ga.ReportRequest{
			// Create the ReportRequest object.
			{
				ViewId: viewId,
				DateRanges: []*ga.DateRange{
					// Create the DateRange object.
					{StartDate: startDate, EndDate: "today"},
				},
				Metrics: []*ga.Metric{
					// Create the Metrics object.
					{Expression: "ga:pageviews"},
				},
				DimensionFilterClauses: []*ga.DimensionFilterClause{
					{
						Filters: []*ga.DimensionFilter{
							{
								Operator:      "BEGINS_WITH",
								DimensionName: "ga:pagePath",
								Expressions: []string{
									"/products/" + strconv.FormatInt(productID, 10),
								},
							},
						},
					},
				},
				Dimensions: []*ga.Dimension{
					{Name: "ga:countryIsoCode"},
				},
				OrderBys: []*ga.OrderBy{
					{
						FieldName: "ga:pageviews",
						SortOrder: "DESCENDING",
					},
				},
			},
		},
	}

	log.Info(log.V{"Google Analytics": "Doing GET request from analytics reporting"})
	// Call the BatchGet method and return the response.
	return svc.Reports.BatchGet(req).Do()
}

// printResponse parses and prints the Analytics Reporting API V4 response.
func printResponse(res *ga.GetReportsResponse) (string, int64, error) {

	var top3Countries strings.Builder
	var totalPageViews int64

	for _, report := range res.Reports {
		header := report.ColumnHeader
		dimHdrs := header.Dimensions
		metricHdrs := header.MetricHeader.MetricHeaderEntries
		rows := report.Data.Rows

		if rows == nil {
			return "", 0, errors.New("No data found for given view.")
		}

		for row_index, row := range rows {
			dims := row.Dimensions
			metrics := row.Metrics

			for i := 0; i < len(dimHdrs) && i < len(dims) && row_index < 3; i++ {
				log.Info(log.V{"Google Analytics, Dimension Header": dimHdrs[i], "Google Analytics, Dimension Value": dims[i]})

				top3Countries.WriteString(dims[i])

				if len(rows)-row_index != 1 && row_index != 2 {
					top3Countries.WriteString(",")
				}

				top3Countries.WriteString(" ")
			}

			for _, metric := range metrics {
				// We have only 1 date range in the example
				// So it'll always print "Date Range (0)"
				for j := 0; j < len(metricHdrs) && j < len(metric.Values); j++ {
					log.Info(log.V{"Google Analytics, Metric Header": metricHdrs[j].Name, "Google Analytics, Metric Value": metric.Values[j]})
					if metricHdrs[j].Name == "ga:pageviews" {
						countryPageViews, _ := strconv.ParseInt(metric.Values[j], 10, 32)
						totalPageViews += countryPageViews
					}
				}
			}
			log.Info(log.V{"Google Analytics, Total page views": totalPageViews})
		}
	}
	log.Info(log.V{"Google Analytics": "Completed printing response"})
	return top3Countries.String(), totalPageViews, nil
}
