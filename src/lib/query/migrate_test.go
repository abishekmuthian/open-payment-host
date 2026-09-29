package query_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/internal/testenv"
	"github.com/abishekmuthian/open-payment-host/src/lib/query"
)

// Columns read by products.NewWithColumns / AllowedParamsAdmin.
var productColumns = []string{
	"id", "created_at", "updated_at", "status", "comment_count", "name", "points", "rank", "summary",
	"description", "featured_image", "url", "s3_bucket", "s3_key", "user_id", "user_name",
	"all_time_page_views", "all_time_top3_countries", "seven_days_page_views", "seven_days_top3_countries",
	"thirty_days_page_views", "thirty_days_top3_countries", "insights_updated", "subscribers",
	"total_subscribers", "total_onetime_payments", "mailchimp_audience_id", "listmonk_list_id",
	"stripe_price", "square_price", "schedule", "square_subscription_plan_id", "paypal_price",
	"razorpay_price", "webhook_url", "webhook_secret", "allowed_redirect_origins",
}

// Columns read by subscriptions.NewWithColumns and written by fulfillment.
var subscriptionColumns = []string{
	"id", "created_at", "updated_at", "txn_id", "payment_status", "item_number", "item_name",
	"mc_currency", "mc_gross", "payer_email", "payer_id", "payment_date", "subscr_id", "user_id",
	"pg", "payer_phone", "custom", "invoice",
}

var paymentTables = []string{"payment_attempts", "payment_events", "payment_transactions", "payment_outbox"}

func TestFreshDatabaseMigratesCleanly(t *testing.T) {
	testenv.Config(t, nil)
	path := testenv.DB(t) // fails unless version == latest and not dirty

	if v, dirty := testenv.MigrationState(t); v != 14 || dirty != 0 {
		t.Fatalf("migration state = %d dirty=%d, want 14 clean", v, dirty)
	}
	products := testenv.Columns(t, "products")
	for _, c := range productColumns {
		if !products[c] {
			t.Errorf("products.%s missing after migration", c)
		}
	}
	subs := testenv.Columns(t, "subscriptions")
	for _, c := range subscriptionColumns {
		if !subs[c] {
			t.Errorf("subscriptions.%s missing after migration", c)
		}
	}
	for _, table := range paymentTables {
		if len(testenv.Columns(t, table)) == 0 {
			t.Errorf("table %s missing after migration", table)
		}
	}
	outbox := testenv.Columns(t, "payment_outbox")
	for _, channel := range []string{"webhook", "mailing"} {
		for _, c := range []string{"_attempts", "_next_attempt_at", "_last_error"} {
			if !outbox[channel+c] {
				t.Errorf("payment_outbox.%s%s missing after migration 000014", channel, c)
			}
		}
	}
	if typ := testenv.Text(t, "SELECT type FROM pragma_table_info('subscriptions') WHERE name='user_id'"); !strings.EqualFold(typ, "text") {
		t.Errorf("subscriptions.user_id type = %q, want text (migration 000009)", typ)
	}

	// Reopening the same file is a no-op: nothing to migrate, still clean.
	testenv.Exec(t, "INSERT INTO products(name) VALUES('kept')")
	if err := query.CloseDatabase(); err != nil {
		t.Fatal(err)
	}
	if err := query.OpenDatabase(map[string]string{"adapter": "sqlite3", "db": path}, &sync.RWMutex{}); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if v, dirty := testenv.MigrationState(t); v != 14 || dirty != 0 {
		t.Fatalf("after reopen = %d dirty=%d", v, dirty)
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM products WHERE name='kept'"); n != 1 {
		t.Fatalf("data lost on reopen: %d", n)
	}
}

func TestOpenDatabaseReportsMigrationFailure(t *testing.T) {
	testenv.Config(t, nil)
	testenv.Root(t)
	path := filepath.Join(t.TempDir(), "broken.db")
	if err := query.OpenDatabase(map[string]string{"adapter": "sqlite3", "db": path}, &sync.RWMutex{}); err != nil {
		t.Fatal(err)
	}
	// Pretend migration 6 never ran although its column exists: re-running it fails.
	testenv.Exec(t, "UPDATE schema_migrations SET version=5, dirty=0")
	query.CloseDatabase()

	err := query.OpenDatabase(map[string]string{"adapter": "sqlite3", "db": path}, &sync.RWMutex{})
	t.Cleanup(func() { query.CloseDatabase() })
	if err == nil {
		t.Fatal("OpenDatabase hid a failed migration")
	}
}

// Installs created from the pre-0.3.10 baseline (which already had paypal_price
// and listmonk_list_id) stopped dirty at version 1; reopening must repair them.
func TestOpenDatabaseRepairsBaselineDirtyInstall(t *testing.T) {
	testenv.Config(t, nil)
	testenv.Root(t)
	path := filepath.Join(t.TempDir(), "stuck.db")
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"CREATE TABLE products (id integer primary key autoincrement, name text, paypal_price text, listmonk_list_id integer DEFAULT 0)",
		"CREATE TABLE schema_migrations (version uint64, dirty bool)",
		"CREATE UNIQUE INDEX version_unique ON schema_migrations (version)",
		"INSERT INTO schema_migrations VALUES (1, 1)",
		"INSERT INTO products(name) VALUES ('existing')",
	} {
		if _, err := raw.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	if err := query.OpenDatabase(map[string]string{"adapter": "sqlite3", "db": path}, &sync.RWMutex{}); err != nil {
		query.CloseDatabase()
		t.Fatalf("stuck install not repaired: %v", err)
	}
	t.Cleanup(func() { query.CloseDatabase() })
	if v, dirty := testenv.MigrationState(t); v != testenv.LatestMigration(t) || dirty != 0 {
		t.Fatalf("after repair = %d dirty=%d", v, dirty)
	}
	if !testenv.Columns(t, "products")["razorpay_price"] || len(testenv.Columns(t, "payment_attempts")) == 0 {
		t.Fatal("later migrations did not run")
	}
	if n := testenv.Scalar(t, "SELECT COUNT(*) FROM products WHERE name='existing'"); n != 1 {
		t.Fatal("existing product lost")
	}
}

func TestOpenDatabaseRejectsUnknownAdapter(t *testing.T) {
	if err := query.OpenDatabase(map[string]string{"adapter": "nope"}, &sync.RWMutex{}); err == nil {
		query.CloseDatabase()
		t.Fatal("unknown adapter accepted")
	}
	query.CloseDatabase()
}
