package listmonk

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-resty/resty/v2"
)

type Subscriber struct {
	ID     int              `json:"id"`
	Email  string           `json:"email"`
	Name   string           `json:"name"`
	Status string           `json:"status"`
	Lists  []SubscriberList `json:"lists"`
}

type SubscriberList struct {
	ID int `json:"id"`
}

type subscriberPayload struct {
	Email                   string                 `json:"email,omitempty"`
	Name                    string                 `json:"name,omitempty"`
	Status                  string                 `json:"status,omitempty"`
	Lists                   []int                  `json:"lists,omitempty"`
	Attribs                 map[string]interface{} `json:"attribs,omitempty"`
	PreconfirmSubscriptions bool                   `json:"preconfirm_subscriptions"`
}

type subscriberResponse struct {
	Data Subscriber `json:"data"`
}

type subscriberSearchResponse struct {
	Data struct {
		Results []Subscriber `json:"results"`
	} `json:"data"`
}

// UpsertSubscriber creates a subscriber or updates an existing subscriber while
// preserving their current list memberships and adding listID.
func UpsertSubscriber(baseURL string, apiToken string, email string, name string, listID int, preconfirmSubscriptions bool) (*Subscriber, error) {
	if baseURL == "" {
		return nil, errors.New("listmonk URL is empty")
	}
	if apiToken == "" {
		return nil, errors.New("listmonk API token is empty")
	}
	if email == "" {
		return nil, errors.New("listmonk subscriber email is empty")
	}
	if listID == 0 {
		return nil, errors.New("listmonk list id is empty")
	}

	subscriber, err := findSubscriberByEmail(baseURL, apiToken, email)
	if err != nil {
		return nil, err
	}
	if subscriber == nil {
		return createSubscriber(baseURL, apiToken, subscriberPayload{
			Email:                   email,
			Name:                    name,
			Status:                  "enabled",
			Lists:                   []int{listID},
			Attribs:                 map[string]interface{}{"UNAME": name},
			PreconfirmSubscriptions: preconfirmSubscriptions,
		})
	}

	return patchSubscriber(baseURL, apiToken, subscriber.ID, subscriberPayload{
		Email:                   email,
		Name:                    name,
		Status:                  "enabled",
		Lists:                   appendListID(subscriber.Lists, listID),
		Attribs:                 map[string]interface{}{"UNAME": name},
		PreconfirmSubscriptions: preconfirmSubscriptions,
	})
}

func authorizationHeader(apiToken string) (string, error) {
	if strings.HasPrefix(strings.ToLower(apiToken), "token ") {
		apiToken = strings.TrimSpace(apiToken[len("token "):])
	}
	if !strings.Contains(apiToken, ":") {
		return "", errors.New("listmonk API token must be in api_key:token format")
	}
	return "token " + apiToken, nil
}

func findSubscriberByEmail(baseURL string, apiToken string, email string) (*Subscriber, error) {
	var result subscriberSearchResponse
	authHeader, err := authorizationHeader(apiToken)
	if err != nil {
		return nil, err
	}
	query := "subscribers.email = '" + strings.ReplaceAll(email, "'", "''") + "'"

	resp, err := resty.New().R().
		SetHeader("Authorization", authHeader).
		SetResult(&result).
		SetQueryParam("page", "1").
		SetQueryParam("per_page", "1").
		SetQueryParam("query", query).
		Get(strings.TrimRight(baseURL, "/") + "/api/subscribers")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("listmonk API returned %s: %s", resp.Status(), resp.String())
	}
	if len(result.Data.Results) == 0 {
		return nil, nil
	}

	return &result.Data.Results[0], nil
}

func createSubscriber(baseURL string, apiToken string, payload subscriberPayload) (*Subscriber, error) {
	var result subscriberResponse
	authHeader, err := authorizationHeader(apiToken)
	if err != nil {
		return nil, err
	}

	resp, err := resty.New().R().
		SetHeader("Content-Type", "application/json; charset=utf-8").
		SetHeader("Authorization", authHeader).
		SetBody(payload).
		SetResult(&result).
		Post(strings.TrimRight(baseURL, "/") + "/api/subscribers")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("listmonk API returned %s: %s", resp.Status(), resp.String())
	}

	return &result.Data, nil
}

func patchSubscriber(baseURL string, apiToken string, subscriberID int, payload subscriberPayload) (*Subscriber, error) {
	var result subscriberResponse
	authHeader, err := authorizationHeader(apiToken)
	if err != nil {
		return nil, err
	}

	resp, err := resty.New().R().
		SetHeader("Content-Type", "application/json; charset=utf-8").
		SetHeader("Authorization", authHeader).
		SetBody(payload).
		SetResult(&result).
		Patch(fmt.Sprintf("%s/api/subscribers/%d", strings.TrimRight(baseURL, "/"), subscriberID))
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("listmonk API returned %s: %s", resp.Status(), resp.String())
	}

	return &result.Data, nil
}

func appendListID(lists []SubscriberList, listID int) []int {
	ids := make([]int, 0, len(lists)+1)
	exists := false
	for _, list := range lists {
		if list.ID == listID {
			exists = true
		}
		ids = append(ids, list.ID)
	}
	if !exists {
		ids = append(ids, listID)
	}

	return ids
}
