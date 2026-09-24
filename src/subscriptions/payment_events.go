package subscriptions

import (
	"database/sql"
	"errors"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"time"
)

func eventExists(tx *query.Tx, gateway, id string) (bool, error) {
	var value string
	err := tx.QueryRow("SELECT event_id FROM payment_events WHERE gateway=? AND event_id=?", gateway, id).Scan(&value)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}
func recordEvent(tx *query.Tx, gateway, id string) error {
	_, err := tx.Exec("INSERT INTO payment_events(id,created_at,gateway,event_id) VALUES(?,?,?,?)", gateway+":"+id, time.Now().UTC().Format(time.RFC3339), gateway, id)
	return err
}

// Processing errors remain retryable. Payment/state effects have their own
// transactional guards, so a crash before recording the event cannot repeat them.
func processPaymentEvent(gateway, id string, process func() error) error {
	if id == "" {
		return errors.New("missing provider event id")
	}
	seen := false
	err := query.Transaction(func(tx *query.Tx) error { var err error; seen, err = eventExists(tx, gateway, id); return err })
	if err != nil || seen {
		return err
	}
	if err := process(); err != nil {
		return err
	}
	return query.Transaction(func(tx *query.Tx) error {
		seen, err := eventExists(tx, gateway, id)
		if err != nil || seen {
			return err
		}
		return recordEvent(tx, gateway, id)
	})
}
