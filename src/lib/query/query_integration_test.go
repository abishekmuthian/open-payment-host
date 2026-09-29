//go:build integration

package query

// PostgreSQL and MySQL adapter tests. They need running databases and are
// excluded from the default hermetic run: go test -tags integration ./src/lib/query/...

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

var User = os.ExpandEnv("$USER")
var Password = os.ExpandEnv("$QUERY_TEST_PASS") // may be blank

// ----------------------------------
// PSQL TESTS
// ----------------------------------

func testPQSetup(t *testing.T) {

	fmt.Println("\n---\nTESTING POSTRGRESQL\n---")

	// First execute sql
	cmd := exec.Command("psql", "-dquery_test", "-f./tests/query_test_pq.sql")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	err := cmd.Start()
	if err != nil {
		t.Skipf("psql unavailable: %v", err)
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

		err = OpenDatabase(options, &sync.RWMutex{})
		if err != nil {
			CloseDatabase()
			t.Skipf("database unavailable: %v", err)
		}
		if err != nil {
			t.Fatalf("DB Error %v", err)
		}

		fmt.Printf("---\nQuery Testing Postgres - query_test DB setup complete as user %s\n---", User)
	}

}

func testPQFind(t *testing.T) {

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

func testPQCount(t *testing.T) {

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
func testPQWhere(t *testing.T) {

	q := PagesQuery().Where("id > ?", 1)
	models, err := PagesFindAll(q)
	if err != nil || len(models) != 2 {
		t.Fatalf(Format, "Where Array", "len 2", fmt.Sprintf("%d", len(models)))
	}

}

func testPQOrder(t *testing.T) {

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

func testPQSelect(t *testing.T) {

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

func testPQUpdateAll(t *testing.T) {

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

func testPQUpdate(t *testing.T) {

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

func testPQCreate(t *testing.T) {

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

func testPQDelete(t *testing.T) {

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

func testPQDeleteAll(t *testing.T) {

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
func testPQTeardown(t *testing.T) {

	err := CloseDatabase()
	if err != nil {
		fmt.Println("Close DB ERROR ", err)
	}
}

// ----------------------------------
// MYSQL TESTS
// ----------------------------------

func testMysqlSetup(t *testing.T) {

	fmt.Println("\n---\nTESTING Mysql\n---")

	// First execute sql

	// read whole the file
	bytes, err := ioutil.ReadFile("./tests/query_test_mysql.sql")
	if err != nil {
		t.Skipf("mysql unavailable: %s", err)
	}
	s := string(bytes)

	cmd := exec.Command("mysql", "-u", "root", "--init-command", s, "query_test")
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	err = cmd.Start()
	if err != nil {
		t.Skipf("mysql unavailable: %s", err)
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

		err = OpenDatabase(options, &sync.RWMutex{})
		if err != nil {
			CloseDatabase()
			t.Skipf("database unavailable: %v", err)
		}
		if err != nil {
			t.Fatalf("\n\n----\nMYSQL DB ERROR:\n%s\n----\n\n", err)
		}

		fmt.Println("---\nQuery Testing Mysql - DB setup complete\n---")
	}

}

func testMysqlFind(t *testing.T) {

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

func testMysqlCount(t *testing.T) {

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

func testMysqlWhere(t *testing.T) {

	var models []*Page
	q := PagesQuery().Where("id > ?", 1)
	models, err := PagesFindAll(q)
	if err != nil || len(models) != 2 {
		t.Fatalf(Format, "Where Array", "len 2", fmt.Sprintf("%d", len(models)))
	}

}

func testMysqlOrder(t *testing.T) {

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

func testMysqlSelect(t *testing.T) {

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

func testMysqlUpdate(t *testing.T) {

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

func testMysqlUpdateAll(t *testing.T) {

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

func testMysqlCreate(t *testing.T) {

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

func testMysqlDelete(t *testing.T) {

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

func testMysqlDeleteAll(t *testing.T) {

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

func testMysqlTeardown(t *testing.T) {

	err := CloseDatabase()
	if err != nil {
		fmt.Println("Close DB ERROR ", err)
	}
}

// runAdapterSuite runs order-dependent steps, skipping all when setup skips.
func runAdapterSuite(t *testing.T, setup func(*testing.T), steps []func(*testing.T), teardown func(*testing.T)) {
	if !t.Run("Setup", setup) {
		t.FailNow()
	}
	if database == nil {
		t.Skip("database unavailable")
	}
	defer t.Run("Teardown", teardown)
	for i, step := range steps {
		t.Run(fmt.Sprintf("step%d", i), step)
	}
}

func TestPostgres(t *testing.T) {
	runAdapterSuite(t, testPQSetup, []func(*testing.T){testPQFind, testPQCount, testPQWhere, testPQOrder, testPQSelect, testPQUpdateAll, testPQUpdate, testPQCreate, testPQDelete, testPQDeleteAll}, testPQTeardown)
}

func TestMysql(t *testing.T) {
	runAdapterSuite(t, testMysqlSetup, []func(*testing.T){testMysqlFind, testMysqlCount, testMysqlWhere, testMysqlOrder, testMysqlSelect, testMysqlUpdate, testMysqlUpdateAll, testMysqlCreate, testMysqlDelete, testMysqlDeleteAll}, testMysqlTeardown)
}
