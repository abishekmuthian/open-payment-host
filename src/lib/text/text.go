// Package text performs text manipulation on html strings
// it is used by projects and comments
package text

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
)

var (
	// Trailing defines optional characters allowed after a url or username
	// this excludes some valid urls but urls are not expected to end
	// in these characters
	trailing = `([\s!?.,]?)`

	// Search for links prefaced by word separators (\s\n\.)
	// i.e. not already in anchors, and replace with auto-link
	//	`\s(https?://.*)[\s.!?]`
	// match urls at start of text or with space before only
	urlRx = regexp.MustCompile(`(\A|[\s]+)(https?://[^\s><]*)` + trailing)

	// Search for \s@name in text and replace with links to username search
	// requires an endpoint that redirects /u/kenny to /users/1 etc.
	userRx = regexp.MustCompile(`(\A|[\s]+)@([^\s!?.,<>]*)` + trailing)

	// Search for project in text and replace with links to project link search
	// requires an endpoint that redirects /projects/1 to /projects/1 etc.
	projectRx = regexp.MustCompile(`(\A|[\s]+)/projects/([^\s!?.,<>]*)` + trailing)

	// Search for trailing <p>\s for ConvertNewlines
	trailingPara = regexp.MustCompile(`<p>\s*\z`)
)

// ConvertNewlines converts \n to paragraph tags
// if the text already contains paragraph tags, return unaltered
func ConvertNewlines(s string) string {
	if strings.Contains(s, "<p>") {
		return s
	}

	// Start with para
	s = "<p>" + s
	// Replace newlines with paras
	s = strings.Replace(s, "\n", "</p><p>", -1)
	// Remove trailing para added in step above
	s = string(trailingPara.ReplaceAll([]byte(s), []byte("")))
	return s
}

// ConvertLinks returns the text with various transformations applied -
// bare links are turned into anchor tags, and @refs are turned into user links.
// this is somewhat fragile, better to parse the html
func ConvertLinks(s string) string {
	bytes := []byte(s)
	// Replace bare links with active links
	bytes = urlRx.ReplaceAll(bytes, []byte(`$1<a href="$2">$2</a>$3`))
	// Replace usernames with links
	bytes = userRx.ReplaceAll(bytes, []byte(`$1<a href="/u/$2">@$2</a>$3`))
	// Replace projects with links
	bytes = projectRx.ReplaceAll(bytes, []byte(`$1<a href="/projects/$2">/projects/$2</a>$3`))
	return string(bytes)
}

// GetLinks checks the http status codes for the URLs in the string
// returns true if status code is within acceptable range or  no urls was found
// returns false if status code is not withFing acceptable range
func GetLinks(s string) bool {
	url := urlRx.FindAllString(s, -1)

	for _, u := range url {
		resp, err := http.Get(strings.Replace(strings.Replace(strings.Replace(strings.Replace(u, "\n", "", -1), "\r", "", -1), "&nbsp;", "", -1), " ", "", -1))
		if err != nil {
			log.Error(log.V{"msg": "Error checking http status for URL " + strings.Replace(strings.Replace(strings.Replace(strings.Replace(u, "\n", "", -1), "\r", "", -1), "&nbsp;", "", -1), " ", "", -1), "error": err})
			return false
		}

		// Print the HTTP Status Code and Status Name
		log.Info(log.V{"msg": "HTTP Status", "HTTP Response Status": resp.StatusCode, "HTTP StatusCode": resp.StatusCode})

		if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
			log.Info(log.V{"msg": "HTTP Status is in the 2xx range"})
			return true
		} else {
			log.Info(log.V{"msg": "HTTP Status is not in the 2xx range"})
			return false
		}
	}

	return true
}

// GetUsernames checks the text for usernames and returns them if found
func GetUsernames(s string) []string {
	userHandleRx := regexp.MustCompile(`(\A|[\s]+)@([^\s!?.,<>]*)`)
	userNames := userHandleRx.FindAllString(s, -1)

	return userNames
}
