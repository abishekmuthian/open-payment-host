package listmonk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestUpsertSubscriberCreatesMissingSubscriber(t *testing.T) {
	var created subscriberPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if got := r.Header.Get("Authorization"); got != "token api-key:secret" {
			t.Fatalf("Authorization header = %q", got)
		}
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"results":[]}}`))
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`{"data":{"id":7,"email":"buyer@example.com","name":"Buyer","status":"enabled","lists":[{"id":4}]}}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	subscriber, err := UpsertSubscriber(server.URL, "api-key:secret", "buyer@example.com", "Buyer", 4, true)
	if err != nil {
		t.Fatal(err)
	}
	if subscriber.ID != 7 {
		t.Fatalf("subscriber ID = %d", subscriber.ID)
	}
	if !reflect.DeepEqual(created.Lists, []int{4}) || !created.PreconfirmSubscriptions {
		t.Fatalf("create payload = %#v", created)
	}
}

func TestUpsertSubscriberPreservesExistingLists(t *testing.T) {
	var updated subscriberPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"data":{"results":[{"id":7,"email":"buyer@example.com","lists":[{"id":2}]}]}}`))
		case http.MethodPatch:
			if r.URL.Path != "/api/subscribers/7" {
				t.Fatalf("patch path = %q", r.URL.Path)
			}
			if err := json.NewDecoder(r.Body).Decode(&updated); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`{"data":{"id":7}}`))
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	_, err := UpsertSubscriber(server.URL, "token api-key:secret", "buyer@example.com", "Buyer", 4, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updated.Lists, []int{2, 4}) {
		t.Fatalf("updated lists = %#v", updated.Lists)
	}
	if updated.PreconfirmSubscriptions {
		t.Fatalf("preconfirm_subscriptions = true, want false")
	}
}

func TestUpsertSubscriberRejectsInvalidToken(t *testing.T) {
	_, err := UpsertSubscriber("https://listmonk.example", "invalid", "buyer@example.com", "Buyer", 4, true)
	if err == nil {
		t.Fatal("expected invalid token error")
	}
}
