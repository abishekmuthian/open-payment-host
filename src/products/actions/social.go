package storyactions

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"

	"github.com/abishekmuthian/open-payment-host/src/lib/twitter"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

// TweetTopStory tweets the top story
func TweetTopStory() {
	log.Log(log.Values{"msg": "Sending top story tweet"})

	// Get the top story which has not been tweeted yet
	q := products.Popular().Limit(1).Order("rank desc, points desc, id desc")

	// Don't fetch old products -  newer than 3 days (we don't look at older products)
	//q.Where("created_at > current_timestamp - interval '3 days'")

	// Don't fetch new products - newer than 2 days
	q.Where("created_at < current_timestamp - interval '2 days'")

	// Don't fetch products that have already been shared
	q.Where("shared IS false")

	// Fetch the products
	results, err := products.FindAll(q)
	if err != nil {
		log.Log(log.Values{"message": "products: error getting top story tweet", "error": err})
		return
	}

	// If no results, fall back to older products which have been tweeted ordered by last tweeted (oldest first)
	/*
		if len(results) == 0 {
			q = products.Where("points > 10").Where("name not ilike '%release%'").Where("name not like 'Event:%'").Order("tweeted_at asc").Limit(1)
			results, err = products.FindAll(q)
			if err != nil {
				log.Log(log.Values{"message": "products: error getting top story tweet", "error": err})
				return
			}
		}
	*/

	if len(results) > 0 {
		Shareproduct(results[0])
	} else {
		log.Log(log.Values{"message": "products: warning no product found for sharing"})
	}

}

// TweetStory tweets the given story
func TweetStory(story *products.Story) {

	// Base url from config
	baseURL := config.Get("root_url")

	// Link to the primary url for this type of story
	url := story.PrimaryURL()

	// Check for relative urls
	if strings.HasPrefix(url, "/") {
		url = baseURL + url
	}

	tweet := fmt.Sprintf("%s #golang %s", story.Name, url)

	// If the tweet will be too long for twitter, use GN url
	if len(tweet) > 140 {
		tweet = fmt.Sprintf("%s #golang %s", story.Name, baseURL+story.ShowURL())
	}

	log.Log(log.Values{"message": "products: sending tweet", "tweet": tweet})

	_, err := twitter.Tweet(tweet)
	if err != nil {
		log.Log(log.Values{"message": "products: error tweeting story", "error": err})
		return
	}

	// Record that this story has been tweeted in db
	params := map[string]string{"tweeted_at": query.TimeString(time.Now().UTC())}
	err = story.Update(params)
	if err != nil {
		log.Log(log.Values{"message": "products: error updating tweeted story", "error": err})
		return
	}

}

// Shareproduct shares the product to social media
func Shareproduct(story *products.Story) {
	// Base url from config
	baseURL := config.Get("root_url")

	// Link to the primary url for this type of story
	url := story.PrimaryURL()

	// Check for relative urls
	if strings.HasPrefix(url, "/") {
		url = baseURL + url
	}

	post := fmt.Sprintf("%s - %s #startupidea #productsolving #innovation", story.Name, url)

	// If the tweet will be too long for twitter, use GN url
	if len(post) > 230 {
		post = fmt.Sprintf("%s - %s #startupidea", story.Name, baseURL+story.ShowURL())
	}

	log.Log(log.Values{"social": "products: sharing product", "message": post})

	message := map[string]interface{}{
		"post":      post,
		"platforms": []string{"twitter", "linkedin", "facebook"},
		"mediaUrls": []string{strings.Replace(url+".png", "/products", "/assets/images/products", 1)},
	}

	sendToAyrShare(message)

	redditOptions := map[string]interface{}{
		"title":       []string{RemoveHashTag(story.Name)}, // required
		"subreddit":   config.Get("subreddit"),             // required (no "/r/" needed)
		"reddit_link": url,                                 // optional: post a link
	}

	message = map[string]interface{}{
		"post":          post,
		"platforms":     []string{"reddit"},
		"redditOptions": redditOptions,
	}

	sendToAyrShare(message)

	// Record that this story has been tweeted in db
	params := map[string]string{"tweeted_at": query.TimeString(time.Now().UTC()), "shared": "true"}
	err := story.Update(params)
	if err != nil {
		log.Log(log.Values{"message": "products: error updating shared story", "error": err})
		return
	}
}

// sendToAyrShare sends the message to AyrShare
func sendToAyrShare(message map[string]interface{}) {
	bytesRepresentation, err := json.Marshal(message)
	if err != nil {
		log.Log(log.Values{"message": "products: error creating post for sharing", "error": err})
	}

	req, _ := http.NewRequest("POST", "https://app.ayrshare.com/api/post",
		bytes.NewBuffer(bytesRepresentation))

	req.Header.Add("Content-Type", "application/json; charset=UTF-8")

	// Live API Key
	req.Header.Add("Authorization",
		config.Get("ayrshare_key"))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Log(log.Values{"message": "products: error sharing story", "error": err})
		return
	}

	body, err := ioutil.ReadAll(res.Body)
	bodyString := string(body)

	fmt.Println(bodyString)

	res.Body.Close()
}
