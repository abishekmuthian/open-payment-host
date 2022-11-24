package mux

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// TestSetup sets up a mux for testing
func TestMain(m *testing.M) {
	testSetup()
	c := m.Run()
	os.Exit(c)
}

var m *Mux

func testSetup() {
	m = New()
	SetDefault(m)
}

func TestGetParams(t *testing.T) {

	// Test a simple route
	m.Add("/users/{id:\\d+}/update", handler)

	// Test basic request
	r := httptest.NewRequest(http.MethodGet, "/users/1/update?foo=bar", nil)

	// Test nil request
	params, err := Params(nil)
	if err == nil {
		t.Errorf("params: error reading nil request")
	}

	params, err = Params(r)
	if err != nil {
		t.Errorf("params: error parsing params")
	}
	if len(params.Values) == 0 {
		t.Errorf("params: error parsing zero params")
	}
	if params.Get("id") != "1" {
		t.Errorf("params: Get error")
	}

	if params.GetStrings("foo")[0] != "bar" {
		t.Errorf("params: GetStrings error")
	}

	if params.GetInt("id") != 1 {
		t.Errorf("params: error parsing int id")
	}
	if params.GetInts("id")[0] != 1 {
		t.Errorf("params: error parsing int id")
	}
	if params.GetUniqueInts("id")[0] != 1 {
		t.Errorf("params: error parsing int id")
	}

	if params.Get("foo") != "bar" {
		t.Errorf("params: error parsing foo query")
	}
	if len(params.Map()) != 2 {
		t.Errorf("params: error getting map")
	}

	// Test a request for id with non-numeric slug
	m.Add("/users/{id:\\d+}", handler)
	r = httptest.NewRequest(http.MethodGet, "/users/991-slug-here", nil)
	params, err = Params(r)
	if err != nil {
		t.Errorf("params: error parsing params")
	}
	if params.GetInt("id") != 991 {
		t.Errorf("params: error parsing int id wanted:%d got:%d", 991, params.GetInt("id"))
	}

}

func TestPost(t *testing.T) {

	// Test a POST Request with form params
	m.Add("/users/create", handler).Post()

	form := url.Values{}
	form.Add("foo", "bar")
	form.Add("debug", "1")
	form.Add("unit_id", "3")
	form.Add("unit_id", "4")
	body := strings.NewReader(form.Encode())
	r := httptest.NewRequest("POST", "/users/create?test=asdf&1=é%30&debug=bar&float=4.0&float=2.0&date=2017-04-04", body)
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	params, err := Params(r)
	if err != nil {
		t.Errorf("params: error parsing form params :%s", err)
	}
	if len(params.Values) == 0 {
		t.Errorf("params: error parsing form zero params")
	}
	// This is from the form
	if params.Get("foo") != "bar" {
		t.Errorf("params: error parsing foo query")
	}
	// First debug param is from url params
	if params.Get("debug") != "bar" {
		t.Errorf("params: error parsing debug query")
	}
	// Let's look at both since we have some duplicates
	if params.Values["debug"][0] != "bar" {
		t.Errorf("params: error parsing debug query")
	}
	// This is from the form
	if params.Values["debug"][1] != "1" {
		t.Errorf("params: error parsing debug query")
	}
	if params.GetIntsString("unit_id") != "3,4" {
		t.Errorf("params: error parsing int ids string")
	}
	if params.GetFloat("float") != 4.0 {
		t.Errorf("params: error parsing float")
	}
	if params.GetFloats("float")[1] != 2.0 {
		t.Errorf("params: error parsing float")
	}
	d, err := params.GetDate("date", "2006-01-02")
	if err != nil || d.Year() != 2017 || d.Month() != 4 {
		t.Errorf("params: error parsing date")
	}
}

func TestPostMultipart(t *testing.T) {

	// Test a POST Request with form params
	m.Add("/users/create", handler).Post()

	// Test a multipart form decodes seamlessly into Files

	// Prepare a new multipart form writer
	var formData bytes.Buffer
	w := multipart.NewWriter(&formData)
	fw, err := w.CreateFormFile("file", "myfile")
	if err != nil {
		t.Fatalf("params: error creating form data")
	}
	fw.Write([]byte("contents of file"))

	ff, err := w.CreateFormField("key")
	if err != nil {
		t.Fatalf("params: error creating form data")
	}
	_, err = ff.Write([]byte("value"))
	if err != nil {
		t.Errorf("params: error creating form data")
	}
	w.Close()

	r := httptest.NewRequest("POST", "/users/create?id=9", &formData)
	r.Header.Set("Content-Type", w.FormDataContentType())

	params, err := Params(r)
	if err != nil {
		t.Fatalf("params: error parsing form params :%s", err)
	}
	if len(params.Values) == 0 {
		t.Errorf("params: error parsing form zero params")
	}

	if params.GetInt("id") != 9 {
		t.Errorf("params: error parsing id from multipart")
	}

	if params.Get("key") != "value" {
		t.Errorf("params: error parsing key from multipart form params:%v files:%v", params.Values, params.Files)
	}
	if len(params.Files) != 1 || params.Files["file"] == nil {
		t.Fatalf("params: error parsing file from files:%v %v", params.Values, params.Files)
	}
	fh := params.Files["file"][0]
	if fh == nil {
		t.Fatalf("params: error parsing file from files:%v", params.Files)
	}
	if fh.Filename != "myfile" {
		t.Errorf("params: error parsing file from files:%v", params.Files)
	}
	// TODO: file is there, verify reading file contents compare with string above
}
