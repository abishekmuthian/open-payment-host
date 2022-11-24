package app

import (
	"time"

	useractions "github.com/abishekmuthian/open-payment-host/src/users/actions"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/twitter"
	storyactions "github.com/abishekmuthian/open-payment-host/src/products/actions"
)

// SetupServices sets up external services from our config file
func SetupServices() {

	// Don't send if not on production server
	if !config.Production() {
		return
	}

	now := time.Now().UTC()

	// Set up twitter if available, and schedule tweets
	if config.Get("twitter_secret") != "" {
		twitter.Setup(config.Get("twitter_key"), config.Get("twitter_secret"), config.Get("twitter_token"), config.Get("twitter_token_secret"))

		//tweetTime := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.UTC)
		tweetInterval := (23 * time.Hour) + (11 * time.Minute)

		// For testing
		tweetTime := now.Add(time.Second * 5)

		ScheduleAt(storyactions.TweetTopStory, tweetTime, tweetInterval)
	}

	// Set up share if available, and schedule tweets
	if config.Get("ayrshare_key") != "" {

		tweetTime := time.Date(now.Year(), now.Month(), now.Day(), 15, 30, 0, 0, time.UTC)
		tweetInterval := (23 * time.Hour) + (11 * time.Minute)

		// For testing
		//tweetTime := now.Add(time.Second * 2)

		ScheduleAt(storyactions.TweetTopStory, tweetTime, tweetInterval)
	}
	/*
		// Set up mail
		if config.Get("mailchimp_template_id") != "" {
			//mail.Setup(config.Get("mail_secret"), config.Get("mail_from"))

			// Schedule emails to go out at 04:00 UTC every monday, starting from the next occurance
			emailTime := time.Date(now.Year(), now.Month(), now.Day(), 16, 00, 00, 00, time.UTC)

			// Today's day of Week
			weekday := emailTime.Weekday()

			// Get how many days until next Monday if today is not a Monday
			if int(weekday) != 1 {
				daysUntilMonday := (1 - int(weekday) + 7) % 7
				emailTime = emailTime.AddDate(0, 0, daysUntilMonday)
			}

			emailInterval := 7 * 24 * time.Hour // Send Emails weekly

			// For testing send immediately on launch
			//emailTime := now.Add(time.Second * 2)

			ScheduleAt(storyactions.EmailTopProducts, emailTime, emailInterval)
		} */

	// Set up Monthly Email
	if config.Get("mailchimp_monthly_insights_template") != "" {
		// Schedule emails to go out at 04:00 UTC every first day of the month, starting from the next occurrence
		emailTime := time.Date(now.Year(), now.Month(), now.Day(), 16, 00, 00, 00, time.UTC)

		// Day of the Month
		day := emailTime.Day()

		// Get how many days until next Month if today is first day of the month
		if day != 1 {
			t := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC)
			lastDayOfTheMonth := t.Day()
			remainingDaysOfTheMonth := lastDayOfTheMonth - day
			emailTime = emailTime.AddDate(0, 0, remainingDaysOfTheMonth+1)
		}

		// FIXME Handle odd/even months to send the email on the first of every month
		emailInterval := 31 * 24 * time.Hour // Send Emails weekly

		// For testing send immediately on launch
		//	emailTime := now.Add(time.Second * 2)
		ScheduleAt(useractions.EmailMonthlyInsights, emailTime, emailInterval)
	}

	// Update insights
	if config.Get("analytics_view_id") != "" {

		// Schedule insights to update at 00:00 UTC every day, starting from the next occurance
		updateTime := time.Date(now.Year(), now.Month(), now.Day(), 23, 55, 00, 00, time.UTC)

		updateInterval := 24 * time.Hour // Schedule every 24 hours

		// For testing send immediately on launch
		//updateTime := now.Add(time.Second * 2)

		ScheduleAt(storyactions.UpdateInsights, updateTime, updateInterval)
	}

}

// ScheduleAt schedules execution for a particular time and at intervals thereafter.
// If interval is 0, the function will be called only once.
// Callers should call close(task) before exiting the app or to stop repeating the action.
func ScheduleAt(f func(), t time.Time, i time.Duration) chan struct{} {
	task := make(chan struct{})
	now := time.Now().UTC()

	// Check that t is not in the past, if it is increment it by interval until it is not
	for now.Sub(t) > 0 {
		t = t.Add(i)
	}

	// We ignore the timer returned by AfterFunc - so no cancelling, perhaps rethink this
	tillTime := t.Sub(now)
	time.AfterFunc(tillTime, func() {
		// Call f at least once at the time specified
		go f()

		// If we have an interval, call it again repeatedly after interval
		// stopping if the caller calls stop(task) on returned channel
		if i > 0 {
			ticker := time.NewTicker(i)
			go func() {
				for {
					select {
					case <-ticker.C:
						go f()
					case <-task:
						ticker.Stop()
						return
					}
				}
			}()
		}
	})

	return task // call close(task) to stop executing the task for repeated tasks
}
