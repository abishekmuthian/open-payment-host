// Package schedule provides a simple way to schedule functions at a time or interval
package schedule

import (
	"time"
)

// Context is the context passed to ScheduledActions, a subset of the router.Context interface
type Context interface {

	// Config returns a key from the context config
	Config(key string) string

	// Production returns true if we are running in a production environment
	Production() bool

	// Store arbitrary data for this request
	Set(key string, data interface{})

	// Retreive arbitrary data for this request
	Get(key string) interface{}

	// Log a message
	Log(message string)

	// Log a format and arguments
	Logf(format string, v ...interface{})
}

// ScheduledAction is the function type passed in to be executed at the given time
type ScheduledAction func(Context)

// At schedules execution for a particular time and at intervals thereafter.
// If interval is 0, the function will be called only once.
// Callers should call close(task) before exiting the app or to stop repeating the action.
func At(f ScheduledAction, context Context, t time.Time, i time.Duration) chan struct{} {
	task := make(chan struct{})
	now := time.Now().UTC()

	// Check that t is not in the past, if it is increment it by interval until it is not
	for now.Sub(t) > 0 {
		t = t.Add(i)
	}

	// Log the first time we are scheduling for
	if !context.Production() {
		context.Logf("schedule: action registered for:%s", t)
	}

	// We ignore the timer returned by AfterFunc - so no cancelling, perhaps rethink this
	tillTime := t.Sub(now)
	time.AfterFunc(tillTime, func() {
		// Call f at least once at the time specified
		go f(context)

		// If we have an interval, call it again repeatedly after interval
		// stopping if the caller calls stop(task) on returned channel
		if i > 0 {
			ticker := time.NewTicker(i)
			go func() {
				for {
					select {
					case <-ticker.C:
						go f(context)
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
