package translation

import (
	"testing"
)

// TestLoad loads our files from this dir (assumes GOPATH set)
func TestLoad(t *testing.T) {
	p := "test_data"
	err := Load(p)
	if err != nil {
		t.Fatalf("Load translations failed at path:%s error:%s", p, err)
	}

}

// TestTranslate tests translations in english and french
func TestTranslate(t *testing.T) {
	en := Get("en", "foo")
	fr := Get("fr", "foo")

	if en != "bar" {
		t.Fatalf("English translation failed:%s expected:%s", en, "bar")
	}

	if fr != "barré" {
		t.Fatalf("French translation failed:%s expected:%s", fr, "barré")
	}
}
