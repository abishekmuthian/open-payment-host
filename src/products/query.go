package products

import (
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/query"

	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"github.com/abishekmuthian/open-payment-host/src/lib/status"
)

const (
	// TableName is the database table for this resource
	TableName = "products"
	// KeyName is the primary key value for this resource
	KeyName = "id"
	// Order defines the default sort order in sql for this resource
	Order = "name asc, id desc"
	// SubscriberColumnName holds the column name of the subscribers
	SubscriberColumnName = "subscribers"
)

// AllowedParams returns the cols editable by everyone
func AllowedParams() []string {
	return []string{"name", "summary", "url", "mailchimp_token", "mailchimp_list_id"}
}

// AllowedParamsAdmin returns the cols editable by admins
func AllowedParamsAdmin() []string {
	return []string{"status", "comment_count", "name", "points", "rank", "summary", "description", "url", "s3_bucket", "s3_key", "user_id", "user_name", "mailchimp_audience_id", "stripe_price", "square_price", "schedule", "square_subscription_plan_Id"}
}

// NewWithColumns creates a new story instance and fills it with data from the database cols provided.
func NewWithColumns(cols map[string]interface{}) *Story {

	story := New()
	story.ID = resource.ValidateInt(cols["id"])
	story.CreatedAt = resource.ValidateTime(cols["created_at"])
	story.UpdatedAt = resource.ValidateTime(cols["updated_at"])
	story.Status = resource.ValidateInt(cols["status"])
	story.CommentCount = resource.ValidateInt(cols["comment_count"])
	story.Name = resource.ValidateString(cols["name"])
	story.Points = resource.ValidateInt(cols["points"])
	story.Rank = resource.ValidateInt(cols["rank"])
	story.Summary = resource.ValidateString(cols["summary"])
	story.Description = resource.ValidateString(cols["description"])
	story.FeaturedImage = resource.ValidateString(cols["featured_image"])
	story.URL = resource.ValidateString(cols["url"])
	story.S3Bucket = resource.ValidateString(cols["s3_bucket"])
	story.S3Key = resource.ValidateString(cols["s3_key"])
	story.UserID = resource.ValidateInt(cols["user_id"])
	story.UserName = resource.ValidateString(cols["user_name"])
	story.AllTimePageViews = resource.ValidateInt(cols["all_time_page_views"])
	story.AllTimeTop3Countries = resource.ValidateString(cols["all_time_top3_countries"])
	story.SevenDaysPageViews = resource.ValidateInt(cols["seven_days_page_views"])
	story.SevenDaysTop3Countries = resource.ValidateString(cols["seven_days_top3_countries"])
	story.ThirtyDaysPageViews = resource.ValidateInt(cols["thirty_days_page_views"])
	story.ThirtyDaysTop3Countries = resource.ValidateString(cols["thirty_days_top3_countries"])
	story.InsightsUpdatedTime = resource.ValidateTime(cols["insights_updated"])
	// FIXME - Need not load subscribers all the time, create separate function
	story.Subscribers = resource.ValidateInt64Array(cols["subscribers"])
	story.MailchimpAudienceID = resource.ValidateString(cols["mailchimp_audience_id"])
	story.StripePrice = resource.ValidateMap(cols["stripe_price"])
	story.SquarePrice = resource.ValidateNestedMap(cols["square_price"])
	story.Schedule = resource.ValidateString(cols["schedule"])
	story.SquareSubscriptionPlanId = resource.ValidateMap(cols["square_subscription_plan_Id"])

	//Flair
	// FIXME - Create and join the flair column
	/* 	storyOwner, err := users.Find(story.UserID)
	   	if err == nil {
	   		if storyOwner.Subscription {
	   			if storyOwner.Plan == config.Get("subscription_plan_name") {
	   				story.Flair = config.Get("subscription_plan_name_subscriber_flair")
	   			}
	   		}
	   	} else {
	   		log.Error(log.V{"Story query flair, Error finding story owner": err})
	   	}
	*/
	return story
}

// New creates and initialises a new story instance.
func New() *Story {
	story := &Story{}
	story.CreatedAt = time.Now()
	story.UpdatedAt = time.Now()
	story.TableName = TableName
	story.KeyName = KeyName
	story.Status = status.Draft
	return story
}

// FindFirst fetches a single story record from the database using
// a where query with the format and args provided.
func FindFirst(format string, args ...interface{}) (*Story, error) {
	result, err := Query().Where(format, args...).FirstResult()
	if err != nil {
		return nil, err
	}
	return NewWithColumns(result), nil
}

// Find fetches a single story record from the database by id.
func Find(id int64) (*Story, error) {
	result, err := Query().Where("id=?", id).FirstResult()
	if err != nil {
		return nil, err
	}
	return NewWithColumns(result), nil
}

// FindAll fetches all story records matching this query from the database.
func FindAll(q *query.Query) ([]*Story, error) {

	// Fetch query.Results from query
	results, err := q.Results()
	if err != nil {
		return nil, err
	}

	// Return an array of products constructed from the results
	var products []*Story
	for _, cols := range results {
		p := NewWithColumns(cols)
		products = append(products, p)
	}

	return products, nil
}

// Popular returns a query for all products with points over a certain threshold
func Popular() *query.Query {
	return Query().Where("(status is NULL OR status=100) AND points > 2") //product is published OR 0 status AND GREATER THAN 2 POINTS
}

// Query returns a new query for products with a default order.
func Query() *query.Query {
	return query.New(TableName, KeyName).Order(Order)
}

// Where returns a new query for products with the format and arguments supplied.
func Where(format string, args ...interface{}) *query.Query {
	return Query().Where(format, args...)
}

// Published returns a query for all products with status >= published.
func Published() *query.Query {
	return Query().Where("status>=?", status.Published)
}

// getTrendingStory returns a story with most number of views in past 30 days except meta stories
func GetTrendingStory() *query.Query {
	return Query().Where("thirty_days_page_views IS NOT NULL AND id NOT IN (1,2,3,4,5)").Order("thirty_days_page_views desc").Limit(1)
}
