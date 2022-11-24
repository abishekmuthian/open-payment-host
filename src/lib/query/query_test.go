package query

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// At present psql and mysql are tested, sqlite is disabled due to cross-compilation requirements

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

// ----------------------------------
// Test Helpers
// ----------------------------------

var User = os.ExpandEnv("$USER")
var Password = os.ExpandEnv("$QUERY_TEST_PASS") // may be blank

var Format = "\n---\nFAILURE\n---\ninput:    %q\nexpected: %q\noutput:   %q"

// ----------------------------------
// PSQL TESTS
// ----------------------------------

func TestPQSetup(t *testing.T) {

	fmt.Println("\n---\nTESTING POSTRGRESQL\n---")

	// First execute sql
	cmd := exec.Command("psql", "-dquery_test", "-f./tests/query_test_pq.sql")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	err := cmd.Start()
	if err != nil {
		t.Fatalf("DB Test Error %v", err)
	}
	io.Copy(os.Stdout, stdout)
	io.Copy(os.Stderr, stderr)
	cmd.Wait()

	if err == nil {
		// Open the database
		options := map[string]string{
			"adapter":  "postgres",
			"user":     User, // Valid username required for databases
			"password": Password,
			"db":       "query_test",
			"debug":    "true",
		}

		err = OpenDatabase(options)
		if err != nil {
			t.Fatalf("DB Error %v", err)
		}

		fmt.Printf("---\nQuery Testing Postgres - query_test DB setup complete as user %s\n---", User)
	}

}

func TestPQFind(t *testing.T) {

	// This should fail, as there is no such page
	p, err := PagesFind(11)
	if err == nil {
		t.Fatalf(Format, "Find(11)", "nil", p, err)
	}

	// This should work
	_, err = PagesFind(1)
	if err != nil {
		t.Fatalf(Format, "Find(1)", "Model object", err)
	}

}

func TestPQCount(t *testing.T) {

	// This should return 3
	count, err := PagesQuery().Count()
	if err != nil || count != 3 {
		t.Fatalf(Format, "Count failed", "3", fmt.Sprintf("%d", count))
	}

	// This should return 2 - test limit ignored
	count, err = PagesQuery().Where("id < 3").Order("id desc").Limit(100).Count()
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
	models, err := PagesFindAll(q)
	if err != nil || len(models) != 2 {
		t.Fatalf(Format, "Where Array after count", "len 2", err)
	}

}
func TestPQWhere(t *testing.T) {

	q := PagesQuery().Where("id > ?", 1)
	models, err := PagesFindAll(q)
	if err != nil || len(models) != 2 {
		t.Fatalf(Format, "Where Array", "len 2", fmt.Sprintf("%d", len(models)))
	}

}

func TestPQOrder(t *testing.T) {

	// Look for pages in reverse order
	q := PagesQuery().Where("id > 1").Order("id desc")
	models, err := PagesFindAll(q)
	if err != nil || len(models) == 0 {
		t.Fatalf(Format, "Order test id desc", "3", fmt.Sprintf("%d", len(models)))
		return
	}

	p := models[0]
	if p.ID != 3 {
		t.Fatalf(Format, "Order test id desc", "3", fmt.Sprintf("%d", p.ID))

	}

	// Look for pages in right order
	q = PagesQuery().Where("id < ?", 10).Where("id < ?", 100).Order("id asc")
	models, err = PagesFindAll(q)
	if err != nil || models == nil {
		t.Fatalf(Format, "Order test id asc", "1", err)
	}

	p = models[0]
	// Check id and created at time are correct
	if p.ID != 1 || time.Since(p.CreatedAt) > time.Second {
		t.Fatalf(Format, "Order test id asc", "1", fmt.Sprintf("%d", p.ID))
	}

}

func TestPQSelect(t *testing.T) {

	var models []*Page
	q := PagesQuery().Select("SELECT id,title from pages").Order("id asc")
	models, err := PagesFindAll(q)
	if err != nil || len(models) == 0 {
		t.Fatalf(Format, "Select error on id,title", "id,title", err)
	}
	p := models[0]
	// Check id and title selected, other values to be zero values
	if p.ID != 1 || p.Title != "Title 1." || len(p.Text) > 0 || p.CreatedAt.Year() > 1 {
		t.Fatalf(Format, "Select id,title", "id,title only", p)
	}

}

// Some more damaging operations we execute at the end,
// to avoid having to reload the db for each test

func TestPQUpdateAll(t *testing.T) {

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

func TestPQUpdate(t *testing.T) {

	p, err := PagesFind(3)
	if err != nil {
		t.Fatalf(Format, "Update could not find model err", "id-3", err)
	}

	// Should really test updates with several strings here
	// Update each model with a different string
	// This does also check if AllowedParams is working properly to clean params
	err = p.Update(map[string]string{"title": "UPDATE 1"})
	if err != nil {
		t.Fatalf(Format, "Error after update", "updated", err)
	}
	// Check it is modified
	p, err = PagesFind(3)

	if err != nil {
		t.Fatalf(Format, "Error after update 1", "updated", err)
	}

	// Check we have an update and the updated at time was set
	if p.Title != "UPDATE 1" || time.Since(p.UpdatedAt) > time.Second {
		t.Fatalf(Format, "Error after update 1 - Not updated properly", "UPDATE 1", p.Title)
	}

}

func TestPQCreate(t *testing.T) {

	params := map[string]string{
		//	"id":		"",
		"title":      "Test 98",
		"text":       "My text",
		"created_at": "REPLACE ME",
		"summary":    "This is my summary",
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
		t.Fatalf(Format, "Create page params mismatch", "Creation", p.Title)
	}

	// Check we have one left
	count, err := PagesQuery().Count()

	if err != nil || count != 4 {
		t.Fatalf(Format, "Count after create", "4", fmt.Sprintf("%d", count))
	}

}

func TestPQDelete(t *testing.T) {

	p, err := PagesFind(3)
	if err != nil {
		t.Fatalf(Format, "Could not find model err", "id-3", err)
	}

	err = p.Delete()
	if err != nil {
		t.Fatalf(Format, "Error after delete", "deleted", err)
	}

	// Check it is gone and we get an error on next find
	p, err = PagesFind(3)

	if !strings.Contains(fmt.Sprintf("%s", err), "No results found") {
		t.Fatalf(Format, "Error after delete 1", "1", err)
	}

}

func TestPQDeleteAll(t *testing.T) {

	err := PagesQuery().Where("id > 1").DeleteAll()
	if err != nil {
		t.Fatalf(Format, "DELETE ALL err", "delete al above 1 records", err)
	}

	// Check we have one left
	count, err := PagesQuery().Count()

	if err != nil || count != 1 {
		t.Fatalf(Format, "Count after delete all above 1", "1", fmt.Sprintf("%d", count))
	}

}

// This test takes some time, so only enable for speed testing
func BenchmarkPQSpeed(t *testing.B) {

	fmt.Println("\n---\nSpeed testing PSQL\n---")

	for i := 0; i < 100000; i++ {
		// ok  	github.com/abishekmuthian/open-payment-host/src/lib/query	20.238s

		var models []*Page
		q := PagesQuery().Select("SELECT id,title from pages").Where("id < i").Order("id asc")
		models, err := PagesFindAll(q)
		if err != nil && models != nil {

		}

		// ok  	github.com/abishekmuthian/open-payment-host/src/lib/query	21.680s
		q = PagesQuery().Select("SELECT id,title from pages").Where("id < i").Order("id asc")
		r, err := q.Results()
		if err != nil && r != nil {

		}

	}

	fmt.Println("\n---\nSpeed testing PSQL END\n---")

}

// NB this test must come last, any tests after this will try to use an invalid database reference
func TestPQTeardown(t *testing.T) {

	err := CloseDatabase()
	if err != nil {
		fmt.Println("Close DB ERROR ", err)
	}
}

// ----------------------------------
// MYSQL TESTS
// ----------------------------------

func TestMysqlSetup(t *testing.T) {

	fmt.Println("\n---\nTESTING Mysql\n---")

	// First execute sql

	// read whole the file
	bytes, err := ioutil.ReadFile("./tests/query_test_mysql.sql")
	if err != nil {
		t.Fatalf("MYSQL DB ERROR: %s", err)
	}
	s := string(bytes)

	cmd := exec.Command("mysql", "-u", "root", "--init-command", s, "query_test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	err = cmd.Start()
	if err != nil {
		t.Fatalf("MYSQL DB ERROR: %s", err)
	}
	io.Copy(os.Stdout, stdout)
	io.Copy(os.Stderr, stderr)
	cmd.Wait()

	if err == nil {

		// Open the database
		options := map[string]string{
			"adapter": "mysql",
			"db":      "query_test",
			"debug":   "true",
		}

		err = OpenDatabase(options)
		if err != nil {
			t.Fatalf("\n\n----\nMYSQL DB ERROR:\n%s\n----\n\n", err)
		}

		fmt.Println("---\nQuery Testing Mysql - DB setup complete\n---")
	}

}

func TestMysqlFind(t *testing.T) {

	// This should work
	p, err := PagesFind(1)
	if err != nil {
		t.Fatalf(Format, "Find(1)", "Model object", p)
	}

	// This should fail, so we check that
	p, err = PagesFind(11)
	if err == nil {
		t.Fatalf(Format, "Find(1)", "Model object", p)
	}

}

func TestMysqlCount(t *testing.T) {

	// This should return 3
	count, err := PagesQuery().Count()
	if err != nil || count != 3 {
		t.Fatalf(Format, "Count failed", "3", fmt.Sprintf("%d", count))
	}

	// This should return 2 - test limit ignored
	count, err = PagesQuery().Where("id < 3").Order("id desc").Limit(100).Count()
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
	var models []*Page
	models, err = PagesFindAll(q)
	if err != nil || len(models) != 2 {
		t.Fatalf(Format, "Where Array after count", "len 2", err)
	}

}

func TestMysqlWhere(t *testing.T) {

	var models []*Page
	q := PagesQuery().Where("id > ?", 1)
	models, err := PagesFindAll(q)
	if err != nil || len(models) != 2 {
		t.Fatalf(Format, "Where Array", "len 2", fmt.Sprintf("%d", len(models)))
	}

}

func TestMysqlOrder(t *testing.T) {

	// Look for pages in reverse order
	var models []*Page
	q := PagesQuery().Where("id > 1").Order("id desc")
	models, err := PagesFindAll(q)
	if err != nil || len(models) == 0 {
		t.Fatalf(Format, "Order test id desc", "3", fmt.Sprintf("%d", len(models)))
		return
	}

	p := models[0]
	if p.ID != 3 {
		t.Fatalf(Format, "Order test id desc", "3", fmt.Sprintf("%d", p.ID))

	}

	// Look for pages in right order
	q = PagesQuery().Where("id < ?", 10).Where("id < ?", 100).Order("id asc")
	models, err = PagesFindAll(q)
	if err != nil || models == nil {
		t.Fatalf(Format, "Order test id asc", "1", err)
	}

	p = models[0]
	if p.ID != 1 {
		t.Fatalf(Format, "Order test id asc", "1", fmt.Sprintf("%d", p.ID))

	}

}

func TestMysqlSelect(t *testing.T) {

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

func TestMysqlUpdate(t *testing.T) {

	p, err := PagesFind(3)
	if err != nil {
		t.Fatalf(Format, "Update could not find model err", "id-3", err)
	}

	// Should really test updates with several strings here
	// Update each model with a different string
	// This does also check if AllowedParams is working properly to clean params
	err = p.Update(map[string]string{"title": "UPDATE 1"})
	if err != nil {
		t.Fatalf(Format, "Error after update", "updated", err)
	}

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

func TestMysqlUpdateAll(t *testing.T) {

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

func TestMysqlCreate(t *testing.T) {

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

	if p.Text != "My text" {
		t.Fatalf(Format, "Create page params mismatch", "Creation", p.ID)
	}

	// Check we have one left
	count, err := PagesQuery().Count()

	if err != nil || count != 4 {
		t.Fatalf(Format, "Count after create", "4", fmt.Sprintf("%d", count))
	}

}

func TestMysqlDelete(t *testing.T) {

	p, err := PagesFind(3)
	if err != nil {
		t.Fatalf(Format, "Could not find model err", "id-3", err)
	}
	err = p.Delete()
	if err != nil {
		t.Fatalf(Format, "Error after delete", "deleted", err)
	}

	// Check it is gone and we get an error on next find
	p, err = PagesFind(3)
	if !strings.Contains(fmt.Sprintf("%s", err), "No results found") {
		t.Fatalf(Format, "Error after delete 1", "1", err)
	}

}

func TestMysqlDeleteAll(t *testing.T) {

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

func TestMysqlTeardown(t *testing.T) {

	err := CloseDatabase()
	if err != nil {
		fmt.Println("Close DB ERROR ", err)
	}
}

/*
// See note in adapters/database_sqlite.go for reasons this is disabled
// ----------------------------------
// SQLITE TESTS
// ----------------------------------

func TestSQSetup(t *testing.T) {

	fmt.Println("\n---\nTESTING SQLITE\n---")

	// NB we use binary named sqlite3 - this is the default on OS X
	// NB this requires sqlite3 version > 3.7.15 for init alternative would be to echo sql file at end
	cmd := exec.Command("sqlite3", "--init", "tests/query_test_sqlite.sql", "tests/query_test.sqlite")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	err = cmd.Start()
	if err != nil {
		fmt.Println("Could not set up sqlite db - ERROR ", err)
		os.Exit(1)
	}
	go io.Copy(os.Stdout, stdout)
	go io.Copy(os.Stderr, stderr)
	cmd.Wait()

	if err == nil {
		_ = strings.Replace("", "", "", -1)

		// Open the database
		options := map[string]string{
			"adapter": "sqlite3",
			"db":      "tests/query_test.sqlite",
			"debug":   "true", // for more detail on failure, enable debug mode on db
		}

		err = OpenDatabase(options)
		if err != nil {
			fmt.Println("Open database ERROR ", err)
			os.Exit(1)
		}

		fmt.Println("---\nQuery Testing Sqlite3 - DB setup complete\n---")
	}

}

func TestSQFind(t *testing.T) {

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

func TestSQCount(t *testing.T) {

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

func TestSQWhere(t *testing.T) {

	q := PagesQuery().Where("id > ?", 1)
	pages, err := PagesFindAll(q)

	if err != nil || len(pages) != 2 {
		t.Fatalf(Format, "Where Array", "len 2", fmt.Sprintf("%d", len(pages)))
	}

}

func TestSQOrder(t *testing.T) {

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

func TestSQSelect(t *testing.T) {

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

func TestSQUpdate(t *testing.T) {

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

func TestSQUpdateAll(t *testing.T) {

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

func TestSQCreate(t *testing.T) {

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

func TestSQDelete(t *testing.T) {

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

func TestSQDeleteAll(t *testing.T) {

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

func TestSQTeardown(t *testing.T) {

	err := CloseDatabase()
	if err != nil {
		fmt.Println("Close DB ERROR ", err)
	}
}
*/
