package app

import (
	"os"
	"sync"
	"time"

	// psql driver - we only use a psql db at the moment
	_ "github.com/lib/pq"

	"github.com/abishekmuthian/open-payment-host/src/lib/query"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
)

// SetupDatabase sets up the db with query given our server config.
func SetupDatabase(mu *sync.RWMutex) {
	defer log.Time(time.Now(), log.V{"msg": "Finished opening database", "db": config.Get("db"), "user": config.Get("db_user")})

	options := map[string]string{
		"adapter":  config.Get("db_adapter"),
		"user":     config.Get("db_user"),
		"password": config.Get("db_pass"),
		"db":       config.Get("db"),
	}

	// Optionally Support remote databases
	if len(os.Getenv("DATABASE_URL")) > 0 {
		options["url"] = os.Getenv("DATABASE_URL")
	} else {
		options["url"] = config.Get("db_url")
	}

	// Ask query to open the database
	err := query.OpenDatabase(options, mu)

	if err != nil {
		log.Fatal(log.V{"msg": "unable to read database", "db": config.Get("db"), "error": err})
		os.Exit(1)
	}

}
