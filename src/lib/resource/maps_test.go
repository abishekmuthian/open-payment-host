package resource

import (
	"reflect"
	"testing"
	"time"
)

func TestValidateMaps(t *testing.T) {
	// Invalid or missing values give empty maps rather than panics.
	for _, in := range []interface{}{nil, "", "not json", `["array"]`, 42, []byte("{")} {
		if len(ValidateMap(in)) != 0 || len(ValidateNestedMap(in)) != 0 {
			t.Errorf("%v decoded to a non-empty map", in)
		}
	}
	for _, in := range []interface{}{`{"DF":"price_1"}`, []byte(`{"DF":"price_1"}`)} {
		if got := ValidateMap(in); !reflect.DeepEqual(got, map[string]string{"DF": "price_1"}) {
			t.Errorf("ValidateMap(%v) = %v", in, got)
		}
	}
	want := map[string]map[string]interface{}{"DF": {"amount": 10.5, "currency": "USD"}}
	for _, in := range []interface{}{`{"DF":{"amount":10.5,"currency":"USD"}}`, []byte(`{"DF":{"amount":10.5,"currency":"USD"}}`)} {
		if got := ValidateNestedMap(in); !reflect.DeepEqual(got, want) {
			t.Errorf("ValidateNestedMap(%v) = %v", in, got)
		}
	}
}

func TestCacheKey(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	a := &Base{ID: 1, TableName: "products", UpdatedAt: at}
	b := &Base{ID: 1, TableName: "products", UpdatedAt: at}
	if a.CacheKey() != b.CacheKey() || len(a.CacheKey()) != 64 {
		t.Fatalf("unstable key %q", a.CacheKey())
	}
	for _, other := range []*Base{
		{ID: 1, TableName: "products", UpdatedAt: at.Add(time.Second)},
		{ID: 2, TableName: "products", UpdatedAt: at},
		{ID: 1, TableName: "users", UpdatedAt: at},
	} {
		if other.CacheKey() == a.CacheKey() {
			t.Errorf("key did not change for %+v", other)
		}
	}
}
