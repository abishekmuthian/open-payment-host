package storyactions

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// HandleGetSuggestion handles the suggestion for the editor
func HandleGetSuggestion(w http.ResponseWriter, r *http.Request) error {
	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// ResponsePayload represents the data sent back to the frontend.
	type ResponsePayload struct {
		Completion string `json:"completion"`
	}

	type Prompt struct {
		Text string `json:"text"`
	}
	type SafetySettings struct {
		Category  string `json:"category"`
		Threshold int    `json:"threshold"`
	}

	type Payload struct {
		Prompt          Prompt           `json:"prompt"`
		Temperature     float64          `json:"temperature"`
		TopK            int              `json:"top_k"`
		TopP            float64          `json:"top_p"`
		CandidateCount  int              `json:"candidate_count"`
		MaxOutputTokens int              `json:"max_output_tokens"`
		StopSequences   []any            `json:"stop_sequences"`
		SafetySettings  []SafetySettings `json:"safety_settings"`
	}

	prompt := Prompt{
		Text: params.Get("text"),
	}

	safetySettings := []SafetySettings{
		{
			Category:  "HARM_CATEGORY_DEROGATORY",
			Threshold: 1,
		},
		{
			Category:  "HARM_CATEGORY_TOXICITY",
			Threshold: 1,
		},
		{
			Category:  "HARM_CATEGORY_VIOLENCE",
			Threshold: 2,
		},
		{
			Category:  "HARM_CATEGORY_SEXUAL",
			Threshold: 2,
		},
		{
			Category:  "HARM_CATEGORY_MEDICAL",
			Threshold: 2,
		},
		{
			Category:  "HARM_CATEGORY_DANGEROUS",
			Threshold: 2,
		},
	}

	data := Payload{
		Prompt:          prompt,
		Temperature:     0.7,
		TopK:            40,
		TopP:            0.95,
		CandidateCount:  1,
		MaxOutputTokens: 1024,
		SafetySettings:  safetySettings,
	}
	payloadBytes, err := json.Marshal(data)
	if err != nil {
		log.Error(log.V{"Suggestion, Error marshalling payload": err})
	}
	body := bytes.NewReader(payloadBytes)

	req, err := http.NewRequest("POST", "https://generativelanguage.googleapis.com/v1beta3/models/text-bison-001:generateText?key="+config.Get("palm_key"), body)
	if err != nil {
		log.Error(log.V{"Suggestion, Error sending request to Google generative language": err})
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Error(log.V{"Suggestion, Error getting response from paLM API": err})
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error(log.V{"paLM io.ReadAll: %v": err})
		return err
	}

	type PalmAPIResponse struct {
		Candidates []struct {
			Output        string `json:"output"`
			SafetyRatings []struct {
				Category    string `json:"category"`
				Probability string `json:"probability"`
			} `json:"safetyRatings"`
		} `json:"candidates"`
	}

	type PalmAPIError struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}

	if resp.StatusCode != 200 {
		var error PalmAPIError

		err = json.Unmarshal(b, &error)

		if err != nil {
			log.Error(log.V{"paLM API error JSON Unmarshall": err})
		}

		log.Info(log.V{"paLM API parsed": error.Error.Message})

		return err
	}

	var suggestion PalmAPIResponse

	err = json.Unmarshal(b, &suggestion)

	if err != nil {
		log.Error(log.V{"paLM APIJSON Unmarshall": err})
	}

	log.Info(log.V{"paLM API parsed": suggestion})

	// Convert markdown to HTML

	suggestionHTML := mdToHTML([]byte(suggestion.Candidates[0].Output))

	// Send back the response
	response := ResponsePayload{
		Completion: string(suggestionHTML),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)

	return err
}

func mdToHTML(md []byte) []byte {
	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	return markdown.Render(doc, renderer)
}
