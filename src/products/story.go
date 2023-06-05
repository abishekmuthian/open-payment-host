// Package products represents the story resource
package products

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/subscriptions"

	"github.com/abishekmuthian/open-payment-host/src/lib/model/file"

	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
	"github.com/abishekmuthian/open-payment-host/src/lib/status"
)

// Story handles saving and retreiving products from the database
type Story struct {
	// resource.Base defines behaviour and fields shared between all resources
	resource.Base

	// status.ResourceStatus defines a status field and associated behaviour
	status.ResourceStatus

	Name          string
	Summary       string
	Description   string
	FeaturedImage string
	URL           string
	S3Bucket      string
	S3Key         string
	UserID        int64
	Points        int64
	Rank          int64
	CommentCount  int64

	// UserName denormalises the user name - pull from users join
	UserName string

	// Insights
	AllTimePageViews        int64
	AllTimeTop3Countries    string
	SevenDaysPageViews      int64
	SevenDaysTop3Countries  string
	ThirtyDaysPageViews     int64
	ThirtyDaysTop3Countries string
	InsightsUpdatedTime     time.Time

	// Subscription
	Flair       string
	Subscribers []int64
	Price       map[string]string

	// Mailchimp
	MailchimpAudienceID string

	//Square
	SquarePrice              map[string]map[string]interface{}
	Schedule                 string
	SquareSubscriptionPlanId map[string]string
}

// Domain returns the domain of the story URL
func (s *Story) Domain() string {
	parts := strings.Split(s.URL, "/")
	if len(parts) > 2 {
		return strings.Replace(parts[2], "www.", "", 1)
	}

	if len(s.URL) > 0 {
		return s.URL
	}

	return config.Get("domain")
}

// ShowAsk returns true if this is a Show: or Ask: story
func (s *Story) ShowAsk() bool {
	return strings.HasPrefix(s.Name, "Show:") || strings.HasPrefix(s.Name, "Ask:")
}

// DestinationURL returns the URL of the story
// if no url is set, it uses the CanonicalURL
func (s *Story) DestinationURL() string {
	// If downvoted, don't publish urls
	if s.Points < 0 {
		return ""
	}
	// If we have an external url, return it
	if s.URL != "" {
		return s.URL
	}
	// If we have an empty url, return the story url instead
	return s.CanonicalURL()
}

// CompleteURL returns combination of protocol, root url and destination url
func (s *Story) CompleteURL() string {
	return config.Get("root_url") + s.DestinationURL()
}

// PermaURL returns the combination of protocol, root url and product number

func (s *Story) PermaURL() string {
	return config.Get("root_url") + s.ShowURL()
}

// PrimaryURL returns the URL to use for this story in lists
// Videos and Show Ask products link to the story
// for other links for now it is the destination
func (s *Story) PrimaryURL() string {
	// If video or show or empty, return story url
	if s.YouTube() || s.ShowAsk() || s.URL == "" {
		return s.CanonicalURL()
	}

	return s.DestinationURL()
}

// CanonicalURL is the canonical URL of the story on this site
// including a slug for seo
func (s *Story) CanonicalURL() string {
	return fmt.Sprintf("/products/%d-%s", s.ID, file.SanitizeName(s.Name))
}

// FileName is the file friendly version of the product name
func (s *Story) FileName() string {
	return fmt.Sprintf("%d-%s", s.ID, file.SanitizeName(s.Name))
}

// GetHashTag gives the hashtags in the name
func (s *Story) GetHashTag() []string {
	var myregex = `#\w*`
	var re = regexp.MustCompile(myregex)

	var hashtags = re.FindAllString(s.Name, -1)
	return hashtags
}

// Code returns true if this is a link to a git repository
// At present we only check for github urls, we should at least check for bitbucket
func (s *Story) Code() bool {
	if strings.Contains(s.URL, "https://github.com") {
		if strings.Contains(s.URL, "/commit/") || strings.Contains(s.URL, "/releases/") || strings.HasSuffix(s.URL, ".md") {
			return false
		}
		return true
	}
	return false
}

// GodocURL returns the godoc.org URL for this story, or empty string if none
func (s *Story) GodocURL() string {
	if s.Code() {
		return strings.Replace(s.URL, "https://github.com", "https://godoc.org/github.com", 1)
	}
	return ""
}

// VetURL returns a URL for goreportcard.com, for code repos
func (s *Story) VetURL() string {
	if s.Code() {
		return strings.Replace(s.URL, "https://github.com/", "http://goreportcard.com/report/", 1)
	}
	return ""
}

// YouTube returns true if this is a youtube video
func (s *Story) YouTube() bool {
	return strings.Contains(s.URL, "youtube.com/watch?v=")
}

// YouTubeURL returns the youtube URL
func (s *Story) YouTubeURL() string {
	url := strings.Replace(s.URL, "https://s.youtube.com", "https://www.youtube.com", 1)
	// https://www.youtube.com/watch?v=sZx3oZt7LVg ->
	// https://www.youtube.com/embed/sZx3oZt7LVg
	url = strings.Replace(url, "watch?v=", "embed/", 1)
	return url
}

// CommentCountDisplay returns the comment count or ellipsis if count is 0
func (s *Story) CommentCountDisplay() string {
	if s.CommentCount > 0 {
		return fmt.Sprintf("%d", s.CommentCount)
	}
	return "…"
}

// NameDisplay returns a title string without hashtags (assumed to be at the end),
// by truncating the title at the first #
func (s *Story) NameDisplay() string {
	if strings.Contains(s.Name, "#") {
		return s.Name[0:strings.Index(s.Name, "#")]
	}
	return s.Name
}

// FeaturedImage returns the featured image of the story
// Not used since its now included in the database
/* func (s *Story) FeaturedImage() string {
	return "/assets/images/products/" + fmt.Sprintf("%d-%s-%s", s.ID, filehelper.SanitizeName(s.Name), "featured_image") + ".png"
} */

// Tags are defined as words beginning with # in the title
// TODO: for speed and clarity we could extract at submit time instead and store in db
func (s *Story) Tags() []string {
	var tags []string
	if strings.Contains(s.Name, "#") {
		// split on " #"
		parts := strings.Split(s.Name, " #")
		tags = parts[1:]
	}
	return tags
}

// Editable returns true if this story is editable.
// products are editable if less than 1 hours old
func (s *Story) Editable() bool {
	return time.Now().Sub(s.CreatedAt) < time.Hour*1
}

// OwnedBy returns true if this user id owns this story.
func (s *Story) OwnedBy(uid int64) bool {
	return uid == s.UserID
}

// NegativePoints returns a negative point score or 0 if points is above 0
func (s *Story) NegativePoints() int64 {
	if s.Points > 0 {
		return 0
	}
	return -s.Points
}

func (s *Story) CountSubscribers() int {
	var subscriberCount int
	// Count the subscribers for Square
	if len(s.SquareSubscriptionPlanId) > 0 {
		for _, planId := range s.SquareSubscriptionPlanId {

			q := subscriptions.Query()

			q.Where(fmt.Sprintf("txn_id = '%s' and payment_status= '%s'", planId, "ACTIVE"))

			subscriptions, err := subscriptions.FindAll(q)

			if err == nil {
				subscriberCount = subscriberCount + len(subscriptions)
			}
		}
	}
	// Count the subscribers for Stripe
	// TODO: Implement subscription cancellation update for Stripe
	q := subscriptions.Query()

	q.Where(fmt.Sprintf("item_number = '%d'", s.ID))

	subscriptions, err := subscriptions.FindAll(q)

	if err == nil {
		subscriberCount = subscriberCount + len(subscriptions)
	}

	return subscriberCount
}
