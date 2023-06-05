package subscriptions

type ErrorModel struct {
	Errors []struct {
		Code     string `json:"code"`
		Detail   string `json:"detail"`
		Field    string `json:"field"`
		Category string `json:"category"`
	} `json:"errors"`
}
