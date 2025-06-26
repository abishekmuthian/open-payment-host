package query

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/query/adapters"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// database is the package global db  - this reference is not exported outside the package.
var database adapters.Database

// TableInfo is a structure to store the data from the .sql file
type TableInfo struct {
	Name      string
	Columns   []string
	DataTypes []string
	Keywords  []string
}

// OpenDatabase opens the database with the given options
func OpenDatabase(opts map[string]string, mu *sync.RWMutex) error {

	// If we already have a db, return it
	if database != nil {
		return fmt.Errorf("query: database already open - %s", database)
	}

	// Assign the db global in query package
	switch opts["adapter"] {
	case "sqlite3":
		database = &adapters.SqliteAdapter{
			Mutex: mu,
		}
	case "mysql":
		database = &adapters.MysqlAdapter{}
	case "postgres":
		database = &adapters.PostgresqlAdapter{}
	default:
		database = nil // fail
	}

	if database == nil {
		return fmt.Errorf("query: database adapter not recognized - %s", opts)
	}

	// Ask the db adapter to open
	err := database.Open(opts)

	if err == nil {
		// Create table if it doesn't exist

		b, err := os.ReadFile("data/db/Create-Tables.sql")

		if err == nil {

			// mattn sqlite3 library doesn't support multiple statements so splitting it

			queries := strings.Split(string(b), "\n\n")

			for _, query := range queries {
				statement, err := database.SQLDB().Prepare(query)

				if err == nil {

					_, err := statement.Exec()
					if err != nil {
						log.Error(log.V{"Error creating database tables": err})
						return err
					}
				} else {
					log.Error(log.V{"Error creating database tables": err})
					return err
				}
			}

			log.Info(log.V{"msg": "Finished creating tables"})

			// Migrate Database
			driver, err := sqlite3.WithInstance(database.SQLDB(), &sqlite3.Config{})
			if err != nil {
				log.Error(log.V{"Database migration, Error creating db instance": err})
			}

			m, err := migrate.NewWithDatabaseInstance("file://db/migrate", "sqlite3", driver)
			if err != nil {
				log.Error(log.V{"Database migration, Error creating migration instance ": err})

			}

			if err := m.Up(); err != nil && err != migrate.ErrNoChange {
				log.Error(log.V{"Database migration, Error migrating ": err})
			} else {
				log.Info(log.V{"msg": "Database migration successful"})
			}

		} else {
			log.Error(log.V{"Unable to read the database file": err})
		}

		// Set max connections for sqlite
		if opts["adapter"] == "sqlite3" {
			SetMaxOpenConns(1)
		}

	} else {
		log.Error(log.V{"Error opening database": err})
	}

	return err
}

// CloseDatabase closes the database opened by OpenDatabase
func CloseDatabase() error {
	var err error
	if database != nil {
		err = database.Close()
		database = nil
	}

	return err
}

// SetMaxOpenConns sets the maximum number of open connections
func SetMaxOpenConns(max int) {
	database.SQLDB().SetMaxOpenConns(max)
}

// QuerySQL executes the given sql Query against our database, with arbitrary args
func QuerySQL(query string, args ...interface{}) (*sql.Rows, error) {
	if database == nil {
		return nil, fmt.Errorf("query: QuerySQL called with nil database")
	}
	results, err := database.Query(query, args...)
	return results, err
}

// ExecSQL executes the given sql against our database with arbitrary args
// NB returns sql.Result - not to be used when rows expected
func ExecSQL(query string, args ...interface{}) (sql.Result, error) {
	if database == nil {
		return nil, fmt.Errorf("query: ExecSQL called with nil database")
	}
	results, err := database.Exec(query, args...)
	return results, err
}

// TimeString returns a string formatted as a time for this db
// if the database is nil, an empty string is returned.
func TimeString(t time.Time) string {
	if database != nil {
		return database.TimeString(t)
	}
	return ""
}
