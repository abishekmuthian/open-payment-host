// Tests for the projects package
package products

import (
	"testing"

	"github.com/abishekmuthian/open-payment-host/src/lib/resource"
)

var testName = "foo"

func TestSetup(t *testing.T) {
	err := resource.SetupTestDatabase(2)
	if err != nil {
		t.Fatalf("projects: Setup db failed %s", err)
	}

}

// Test Create method
func TestCreateprojects(t *testing.T) {
	storyParams := map[string]string{
		"name":   testName,
		"status": "100",
	}

	id, err := New().Create(storyParams)
	if err != nil {
		t.Fatalf("projects: Create story failed :%s", err)
	}

	story, err := Find(id)
	if err != nil {
		t.Fatalf("projects: Create story find failed")
	}

	if story.Name != testName {
		t.Fatalf("projects: Create story name failed expected:%s got:%s", testName, story.Name)
	}

}

// Test Index (List) method
func TestListprojects(t *testing.T) {

	// Get all projects (we should have at least one)
	results, err := FindAll(Query())
	if err != nil {
		t.Fatalf("projects: List no story found :%s", err)
	}

	if len(results) < 1 {
		t.Fatalf("projects: List no projects found :%s", err)
	}

}

// Test Update method
func TestUpdateprojects(t *testing.T) {

	// Get the last story (created in TestCreateprojects above)
	story, err := FindFirst("name=?", testName)
	if err != nil {
		t.Fatalf("projects: Update no story found :%s", err)
	}

	name := "bar"
	storyParams := map[string]string{"name": name}
	err = story.Update(storyParams)
	if err != nil {
		t.Fatalf("projects: Update story failed :%s", err)
	}

	// Fetch the story again from db
	story, err = Find(story.ID)
	if err != nil {
		t.Fatalf("projects: Update story fetch failed :%s", story.Name)
	}

	if story.Name != name {
		t.Fatalf("projects: Update story failed :%s", story.Name)
	}

}

// TestQuery tests trying to find published resources
func TestQuery(t *testing.T) {

	results, err := FindAll(Published())
	if err != nil {
		t.Fatalf("projects: error getting projects :%s", err)
	}
	if len(results) == 0 {
		t.Fatalf("projects: published projects not found :%s", err)
	}

	results, err = FindAll(Query().Where("id>=? AND id <=?", 0, 100))
	if err != nil || len(results) == 0 {
		t.Fatalf("projects: no story found :%s", err)
	}
	if len(results) > 2 {
		t.Fatalf("projects: too many projects:%s", err)
	}

}

// Test Destroy method
func TestDestroyprojects(t *testing.T) {

	results, err := FindAll(Query())
	if err != nil || len(results) == 0 {
		t.Fatalf("projects: Destroy no story found :%s", err)
	}
	story := results[0]
	count := len(results)

	err = story.Destroy()
	if err != nil {
		t.Fatalf("projects: Destroy story failed :%s", err)
	}

	// Check new length of projects returned
	results, err = FindAll(Query())
	if err != nil {
		t.Fatalf("projects: Destroy error getting results :%s", err)
	}

	// length should be one less than previous
	if len(results) != count-1 {
		t.Fatalf("projects: Destroy story count wrong :%d", len(results))
	}

}

// TestAllowedParams should always return some params
func TestAllowedParams(t *testing.T) {
	if len(AllowedParams()) == 0 {
		t.Fatalf("projects: no allowed params")
	}
}
