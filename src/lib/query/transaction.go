package query

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Transaction commits all database effects together, rolling back on any error.
// The callback must use tx exclusively, not the package-level connection pool.
func Transaction(fn func(*Tx) error) error {
	if database == nil {
		return errors.New("database is not open")
	}
	tx, err := database.SQLDB().BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = fn(&Tx{tx}); err != nil {
		return err
	}
	return tx.Commit()
}

type Tx struct{ tx *sql.Tx }

func placeholders(statement string, n int) string {
	for i := 0; i < n; i++ {
		statement = strings.Replace(statement, "?", database.Placeholder(i+1), 1)
	}
	return statement
}
func (t *Tx) Exec(statement string, args ...interface{}) (sql.Result, error) {
	return t.tx.Exec(placeholders(statement, len(args)), args...)
}
func (t *Tx) QueryRow(statement string, args ...interface{}) *sql.Row {
	return t.tx.QueryRow(placeholders(statement, len(args)), args...)
}
