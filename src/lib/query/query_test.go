package query

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// SQLite is tested here against a temporary database; PostgreSQL and MySQL
// tests live in query_integration_test.go behind the integration build tag.

// Pages is a simple example model for testing the query package which stores some fields in the db.
// All functions prefixed with Pages here - normally the model would be in a separate function
// so pages.Find(1) etc

// Page model
type Page struct {
	ID        int64
	UpdatedAt time.Time
	CreatedAt time.Time

	OtherField map[string]string
	Title      string
	Summary    string
	Text       string

	UnusedField int8
}

// Create a model object, called from actions.
func (p *Page) Create(params map[string]string) (int64, error) {
	params["created_at"] = TimeString(time.Now().UTC())
	params["updated_at"] = TimeString(time.Now().UTC())
	return PagesQuery().Insert(params)
}

// Update this model object, called from actions.
func (p *Page) Update(params map[string]string) error {
	params["updated_at"] = TimeString(time.Now().UTC())
	return PagesQuery().Where("id=?", p.ID).Update(params)
}

// Delete this page
func (p *Page) Delete() error {
	return PagesQuery().Where("id=?", p.ID).Delete()
}

// NewWithColumns creates a new page instance and fills it with data from the database cols provided
func PagesNewWithColumns(cols map[string]interface{}) *Page {

	page := PagesNew()

	// Normally you'd validate col values with something like the model/validate pkg
	// we'll use a simple dummy function instead
	page.ID = cols["id"].(int64)
	if cols["created_at"] != nil {
		page.CreatedAt = cols["created_at"].(time.Time)
	}
	if cols["updated_at"] != nil {
		page.UpdatedAt = cols["updated_at"].(time.Time)
	}

	if cols["title"] != nil {
		page.Title = cols["title"].(string)
	}
	if cols["summary"] != nil {
		page.Summary = cols["summary"].(string)
	}
	if cols["text"] != nil {
		page.Text = cols["text"].(string)
	}

	return page
}

// New initialises and returns a new Page
func PagesNew() *Page {
	page := &Page{}
	return page
}

// Query returns a new query for pages
func PagesQuery() *Query {
	return New("pages", "id")
}

func PagesFind(ID int64) (*Page, error) {
	result, err := PagesQuery().Where("id=?", ID).FirstResult()
	if err != nil {
		return nil, err
	}
	return PagesNewWithColumns(result), nil
}

func PagesFindAll(q *Query) ([]*Page, error) {
	results, err := q.Results()
	if err != nil {
		return nil, err
	}

	var models []*Page
	for _, r := range results {
		m := PagesNewWithColumns(r)
		models = append(models, m)
	}

	return models, nil
}

var Format = "\n---\nFAILURE\n---\ninput:    %q\nexpected: %q\noutput:   %q"

// ----------------------------------
// SQLITE TESTS
// ----------------------------------

// TestSQLite runs the adapter suite against a temporary SQLite database
// seeded from tests/query_test_sqlite.sql. Steps share state and run in order.
func TestSQLite(t *testing.T) {
	if err := OpenDatabase(map[string]string{"adapter": "sqlite3", "db": filepath.Join(t.TempDir(), "query_test.sqlite")}, &sync.RWMutex{}); err != nil {
		CloseDatabase()
		t.Fatal(err)
	}
	defer CloseDatabase()
	fixture, err := os.ReadFile("tests/query_test_sqlite.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range strings.Split(string(fixture), ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}
		if _, err := ExecSQL(statement); err != nil {
			t.Fatalf("fixture %q: %v", statement, err)
		}
	}
	for _, step := range []struct {
		name string
		fn   func(*testing.T)
	}{
		{"Find", testSQFind}, {"Count", testSQCount}, {"Where", testSQWhere}, {"Order", testSQOrder},
		{"Select", testSQSelect}, {"Update", testSQUpdate}, {"UpdateAll", testSQUpdateAll},
		{"Create", testSQCreate}, {"Delete", testSQDelete}, {"DeleteAll", testSQDeleteAll},
	} {
		if !t.Run(step.name, step.fn) {
			return
		}
	}
}

func TestNilDatabaseErrors(t *testing.T) {
	CloseDatabase()
	if _, err := QuerySQL("SELECT 1"); err == nil {
		t.Fatal("QuerySQL with nil database succeeded")
	}
	if _, err := ExecSQL("SELECT 1"); err == nil {
		t.Fatal("ExecSQL with nil database succeeded")
	}
	if TimeString(time.Now()) != "" {
		t.Fatal("TimeString with nil database not empty")
	}
}
func testSQFind(t *testing.T) {

	// This should work - NB in normal usage this would be query.New
	p, err := PagesFind(1)
	if err != nil {
		t.Fatalf(Format, "Find(1)", "Model object", err)
	}
	// Check we got the page we expect
	if p.ID != 1 {
		t.Fatalf(Format, "Find(1) p", "Model object", p)
	}

	// This should fail, so we check that
	p, err = PagesFind(11)
	if err == nil || p != nil {
		t.Fatalf(Format, "Find(11)", "Model object", err)
	}

}

func testSQCount(t *testing.T) {

	// This should return 3
	count, err := PagesQuery().Count()
	if err != nil || count != 3 {
		t.Fatalf(Format, "Count failed", "3", fmt.Sprintf("%d", count))
	}

	// This should return 2 - test limit ignored
	count, err = PagesQuery().Where("id in (?,?)", 1, 2).Order("id desc").Limit(100).Count()
	if err != nil || count != 2 {
		t.Fatalf(Format, "Count id < 3 failed", "2", fmt.Sprintf("%d", count))
	}

	// This should return 0
	count, err = PagesQuery().Where("id > 3").Count()
	if err != nil || count != 0 {
		t.Fatalf(Format, "Count id > 3 failed", "0", fmt.Sprintf("%d", count))
	}

	// Test retrieving an array, then counting, then where
	// This should work
	q := PagesQuery().Where("id > ?", 1).Order("id desc")

	count, err = q.Count()
	if err != nil || count != 2 {
		t.Fatalf(Format, "Count id > 1 failed", "2", fmt.Sprintf("%d", count), err)
	}

	// Reuse same query to get array after count
	results, err := q.Results()
	if err != nil || len(results) != 2 {
		t.Fatalf(Format, "Where Array after count", "len 2", err)
	}

}

func testSQWhere(t *testing.T) {

	q := PagesQuery().Where("id > ?", 1)
	pages, err := PagesFindAll(q)

	if err != nil || len(pages) != 2 {
		t.Fatalf(Format, "Where Array", "len 2", fmt.Sprintf("%d", len(pages)))
	}

}

func testSQOrder(t *testing.T) {

	// Look for pages in reverse order
	var models []*Page
	q := PagesQuery().Where("id > 0").Order("id desc")
	models, err := PagesFindAll(q)

	if err != nil || len(models) == 0 {
		t.Fatalf(Format, "Order count test id desc", "3", fmt.Sprintf("%d", len(models)))
		return
	}

	p := models[0]
	if p.ID != 3 {
		t.Fatalf(Format, "Order test id desc 1", "3", fmt.Sprintf("%d", p.ID))
		return
	}

	// Look for pages in right order - reset models
	q = PagesQuery().Where("id < ?", 10).Where("id < ?", 100).Order("id asc")
	models, err = PagesFindAll(q)
	//   fmt.Println("TESTING MODELS %v",models)

	if err != nil || models == nil {
		t.Fatalf(Format, "Order test id asc count", "1", err)
	}

	p = models[0]
	if p.ID != 1 {
		t.Fatalf(Format, "Order test id asc 1", "1", fmt.Sprintf("%d", p.ID))
		return
	}

}

func testSQSelect(t *testing.T) {

	var models []*Page
	q := PagesQuery().Select("SELECT id,title from pages").Order("id asc")
	models, err := PagesFindAll(q)
	if err != nil || len(models) == 0 {
		t.Fatalf(Format, "Select error on id,title", "id,title", err)
	}
	p := models[0]
	if p.ID != 1 || p.Title != "Title 1." || len(p.Text) > 0 {
		t.Fatalf(Format, "Select id,title", "id,title only", p)
	}

}

func testSQUpdate(t *testing.T) {

	p, err := PagesFind(3)
	if err != nil {
		t.Fatalf(Format, "Update could not find model err", "id-3", err)
	}

	// Should really test updates with several strings here
	err = p.Update(map[string]string{"title": "UPDATE 1", "summary": "Test summary"})

	// Check it is modified
	p, err = PagesFind(3)

	if err != nil {
		t.Fatalf(Format, "Error after update 1", "updated", err)
	}

	if p.Title != "UPDATE 1" {
		t.Fatalf(Format, "Error after update 1 - Not updated properly", "UPDATE 1", p.Title)
	}

}

// Some more damaging operations we execute at the end,
// to avoid having to reload the db for each test

func testSQUpdateAll(t *testing.T) {

	err := PagesQuery().UpdateAll(map[string]string{"title": "test me"})
	if err != nil {
		t.Fatalf(Format, "UPDATE ALL err", "udpate all records", err)
	}

	// Check we have all pages with same title
	count, err := PagesQuery().Where("title=?", "test me").Count()

	if err != nil || count != 3 {
		t.Fatalf(Format, "Count after update all", "3", fmt.Sprintf("%d", count))
	}

}

func testSQCreate(t *testing.T) {

	params := map[string]string{
		"title":      "Test 98",
		"text":       "My text",
		"created_at": "REPLACE ME",
		"summary":    "me",
	}

	// if your model is in a package, it could be pages.Create()
	// For now to mock we just use an empty page
	id, err := (&Page{}).Create(params)
	if err != nil {
		t.Fatalf(Format, "Err on create", err)
	}

	// Now find the page and test it
	p, err := PagesFind(id)
	if err != nil {
		t.Fatalf(Format, "Err on create find", err)
	}

	if p.Title != "Test 98" {
		t.Fatalf(Format, "Create page params mismatch", "Creation", p.ID)
	}

	// Check we have one left
	count, err := PagesQuery().Count()

	if err != nil || count != 4 {
		t.Fatalf(Format, "Count after create", "4", fmt.Sprintf("%d", count))
	}

}

func testSQDelete(t *testing.T) {

	p, err := PagesFind(3)
	if err != nil {
		t.Fatalf(Format, "Could not find model err", "id-3", err)
	}

	err = p.Delete()

	// Check it is gone and we get an error on next find
	p, err = PagesFind(3)

	if !strings.Contains(fmt.Sprintf("%s", err), "No results found") {
		t.Fatalf(Format, "Error after delete 1", "1", err)
	}

}

func testSQDeleteAll(t *testing.T) {

	err := PagesQuery().Where("id > 1").DeleteAll()
	if err != nil {
		t.Fatalf(Format, "DELETE ALL err", "delete 2 records", err)
	}

	// Check we have one left
	count, err := PagesQuery().Where("id > 0").Count()

	if err != nil || count != 1 {
		t.Fatalf(Format, "Count after delete all", "1", fmt.Sprintf("%d", count))
	}

}
