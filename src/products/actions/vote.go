package storyactions

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"
	"github.com/abishekmuthian/open-payment-host/src/users"
)

// HandleFlag handles POST to /products/123/flag
func HandleFlag(w http.ResponseWriter, r *http.Request) error {

	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	user := session.CurrentUser(w, r)
	ip := getUserIP(r)

	// Check we have no votes already from this user, if we do fail
	if storyHasUserFlag(story, user) {
		return server.NotAuthorizedError(err, "Flag Failed", "Sorry you are not allowed to flag twice, nice try!")
	}

	// Authorise flagging
	if !user.CanFlag() {
		return server.NotAuthorizedError(err, "Flag Failed", "Sorry, you can't flag yet")
	}
	// Flags are more expensive than downvotes
	err = adjustUserPoints(user, -2)
	if err != nil {
		return server.InternalError(err, "Failed to adjust points")
	}

	// Record the flag separately
	delta := int64(-5)
	err = recordStoryFlag(story, user, ip, delta)
	if err != nil {
		return server.InternalError(err, "Failed to add vote")
	}

	// Downvote the story massively - note this adds a vote record already
	err = addStoryVote(story, user, ip, delta)
	if err != nil {
		return server.InternalError(err, "Failed to add vote")
	}

	err = updateproductsRank()
	if err != nil {
		return server.InternalError(err, "Failed to update story rank")
	}

	return server.Redirect(w, r, story.ShowURL())
}

// HandleDownvote handles POST to /products/123/downvote
func HandleDownvote(w http.ResponseWriter, r *http.Request) error {

	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	user := session.CurrentUser(w, r)
	ip := getUserIP(r)

	if !user.Admin() {
		// Check we have no votes already from this user, if we do fail
		if storyHasUserVote(story, user) {
			return server.NotAuthorizedError(err, "Vote Failed", "Sorry you are not allowed to vote twice, nice try!")
		}
	}

	// Authorise downvote on story for this user - our rules are:
	if !user.CanDownvote() {
		return server.NotAuthorizedError(err, "Vote Failed", "Sorry, you can't downvote yet")
	}

	err = adjustUserPoints(user, -1)
	if err != nil {
		return err
	}

	// Adjust points on story and add to the vote table
	err = addStoryVote(story, user, ip, -1)
	if err != nil {
		return err
	}

	return updateproductsRank()
}

// HandleUpvote handles POST to /products/123/upvote
func HandleUpvote(w http.ResponseWriter, r *http.Request) error {

	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the story
	story, err := products.Find(params.GetInt("id"))
	if err != nil {
		return server.NotFoundError(err)
	}

	user := session.CurrentUser(w, r)
	ip := getUserIP(r)

	// Admins can bypass upvote checks
	if !user.Admin() {
		// Check we have no votes already from this user, if we do fail
		if storyHasUserVote(story, user) {
			return server.NotAuthorizedError(err, "Vote Failed", "Sorry you are not allowed to vote twice, nice try!")
		}

	}

	// Authorise upvote on story for this user - our rules are:
	if !user.CanUpvote() {
		return server.NotAuthorizedError(err, "Vote Failed", "Sorry, you can't upvote yet")
	}

	// Adjust points on story and add to the vote table
	err = addStoryVote(story, user, ip, +1)
	if err != nil {
		return err
	}

	return updateproductsRank()
}

// addStoryVote adjusts the story points, and adds a vote record for this user
func addStoryVote(story *products.Story, user *users.User, ip string, delta int64) error {

	if story.Points < -5 && delta < 0 {
		return server.InternalError(nil, "Vote Failed", "Story is already hidden")
	}

	// Update the story points by delta
	err := story.Update(map[string]string{"points": fmt.Sprintf("%d", story.Points+delta)})
	if err != nil {
		return server.InternalError(err, "Vote Failed", "Sorry your adjust vote points")
	}

	// Update the *story* posting user points by delta
	storyUser, err := users.Find(story.UserID)
	if err != nil {
		return err
	}
	err = adjustUserPoints(storyUser, delta)
	if err != nil {
		return err
	}

	return recordStoryVote(story, user, ip, delta)
}

// removeUserPoints removes these points from this user
func adjustUserPoints(user *users.User, delta int64) error {

	// Update the user points
	err := user.Update(map[string]string{"points": fmt.Sprintf("%d", user.Points+delta)})
	if err != nil {
		return server.InternalError(err, "Vote Failed", "Sorry could not adjust user points")
	}

	return nil
}

// recordStoryVote adds a vote record for this user
func recordStoryVote(story *products.Story, user *users.User, IP string, delta int64) error {

	// Add an entry in the votes table
	// FIXME: adjust query to do this for us we should use ?,?,? here...
	// $1, $2 is surprising, shouldn't we expect query package to deal with this for us?
	now := query.TimeString(time.Now().UTC())
	_, err := query.Exec("insert into votes VALUES($1,NULL,$2,$3,$4,$5)", now, story.ID, user.ID, IP, delta)
	if err != nil {
		return server.InternalError(err, "Vote Failed", "Sorry your vote failed to record")
	}

	return nil
}

// storyHasUserVote returns true if we already have a vote for this story from this user
func storyHasUserVote(story *products.Story, user *users.User) bool {
	// Query votes table for rows with userId and storyId
	// if we don't get error, return true
	results, err := query.New("votes", "story_id").Where("story_id=?", story.ID).Where("user_id=?", user.ID).Results()

	if err == nil && len(results) == 0 {
		return false
	}

	return true
}

// recordStoryFlag adds a flag record for this user
func recordStoryFlag(story *products.Story, user *users.User, IP string, delta int64) error {

	// Add an entry in the votes table
	// FIXME: adjust query to do this for us we should use ?,?,? here...
	// $1, $2 is surprising, shouldn't we expect query package to deal with this for us?
	now := query.TimeString(time.Now().UTC())
	_, err := query.Exec("insert into flags VALUES($1,NULL,$2,$3,$4,$5)", now, story.ID, user.ID, IP, delta)
	if err != nil {
		return server.InternalError(err, "Flag Failed", "Sorry your flag failed to record")
	}

	return nil
}

// storyHasUserFlag returns true if we already have a flag for this story from this user
func storyHasUserFlag(story *products.Story, user *users.User) bool {
	// Query flags table for rows with userId and storyId
	// if we don't get error, return true
	results, err := query.New("flags", "story_id").Where("story_id=?", story.ID).Where("user_id=?", user.ID).Results()
	if err == nil && len(results) == 0 {
		return false
	}

	return true
}

func getUserIP(r *http.Request) string {
	// Store a hash of the ip (should we strip port?)
	ip := r.RemoteAddr
	forward := r.Header.Get("X-Forwarded-For")
	if len(forward) > 0 {
		ip = forward
	}

	clientIP := r.Header.Get("CF-Connecting-IP")

	// Hash for anonymity in our store
	hasher := sha256.New()
	if !config.Production() {
		hasher.Write([]byte(ip)) // using localhost for development
	} else {
		hasher.Write([]byte(clientIP)) // using IP from cloudflare for production
	}
	return base64.URLEncoding.EncodeToString(hasher.Sum(nil))
}

// updateproductsRank updates the rank of all products with a rank based on their point score / time elapsed (as represented by id)
//
//	to the power of gravity
//	  update products set rank = points / power((select count(*) from products) - id + 1,1.8);
//
// Similar to HN ranking scheme
func updateproductsRank() error {
	sql := "update products set rank = 100 * points / power((select max(id) from products) - id + 1,1.2)"
	_, err := query.Exec(sql)
	return err
}
