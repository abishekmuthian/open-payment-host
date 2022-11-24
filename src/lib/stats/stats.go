package stats

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
)

// Put this in a separate package called stats

// PurgeInterval is the interval at which users are purged from the current list
var PurgeInterval = time.Minute * 5

// identifiers holds a hash of anonymised user records
// obviously an in-memory store is not suitable for very large sites
// but for smaller sites with a few hundred concurrent users it's fine
var identifiers = make(map[string]time.Time)
var mu sync.RWMutex

// RegisterHit registers a hit and ups user count if required
func RegisterHit(r *http.Request) {

	// Use UA as well as ip for unique values per browser session
	ua := r.Header.Get("User-Agent")
	// Ignore obvious bots (Googlebot etc)
	if strings.Contains(ua, "bot") {
		return
	}
	// Ignore requests for xml (assumed to be feeds or sitemap)
	if strings.HasSuffix(r.URL.Path, ".xml") {
		return
	}

	// Extract the IP from the address
	ip := r.RemoteAddr
	forward := r.Header.Get("X-Forwarded-For")
	if len(forward) > 0 {
		ip = forward
	}

	clientIP := r.Header.Get("CF-Connecting-IP")
	clientCountry := r.Header.Get("CF-IPCountry")

	log.Info(log.V{"Client IP Address": clientIP, "Client Country": clientCountry})

	// Hash for anonymity in our store
	hasher := sha1.New()
	if !config.Production() {
		hasher.Write([]byte(ip)) // using localhost for development
	} else {
		hasher.Write([]byte(clientIP)) // using IP from cloudflare for production
	}
	hasher.Write([]byte(ua))
	id := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

	// Send hits to GA
	payload := url.Values{
		"v":   {"1"},                                 // protocol version = 1
		"t":   {"pageview"},                          // hit type
		"tid": {config.Get("analytics_property_id")}, // tracking / property ID
		"cid": {id},                                  // unique client ID (server generated UUID)
		"dp":  {r.URL.Path},                          // page path
		"uip": {clientIP},                            // IP address of the user
	}

	go sendToGA(ua, clientIP, id, payload)

	// Insert the entry with current time
	mu.Lock()
	identifiers[id] = time.Now()
	mu.Unlock()
}

// sendToGA sends the analytics to GA
func sendToGA(ua string, ip string, cid string, values url.Values) {

	client := &http.Client{}

	req, _ := http.NewRequest("POST", config.Get("analytics_URL"), strings.NewReader(values.Encode()))
	req.Header.Add("User-Agent", ua)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	if resp, err := client.Do(req); err != nil {
		log.Error(log.V{"GA collector POST error": err.Error()})
	} else {
		log.Info(log.V{"\nGA collector status": resp.Status, "\nGA collector cid": cid, "\nGA collector ip": ip})

		// GA sends response body only on debug server, which is set in config for development only
		if !config.Production() {
			var j interface{}
			err = json.NewDecoder(resp.Body).Decode(&j)
			if err != nil {
				log.Error(log.V{"Error parsing GA collector response": err})
			}
			log.Info(log.V{"GA collector response": j})
		}

		log.Info(log.V{"Reported payload": values})
	}
}

// HandleUserCount serves a get request at /stats/users/count
func HandleUserCount(w http.ResponseWriter, r *http.Request) error {

	// Render json of our count for the javascript to display
	mu.RLock()
	json := fmt.Sprintf("{\"users\":%d}", len(identifiers))
	mu.RUnlock()
	_, err := w.Write([]byte(json))
	return err
}

// UserCount returns a count of users in the last 5 minutes
func UserCount() int {
	mu.RLock()
	defer mu.RUnlock()
	return len(identifiers)
}

// Clean up users list at intervals
func init() {
	purgeUsers()
}

// purgeUsers clears the users list of users who last acted PurgeInterval ago
func purgeUsers() {

	mu.Lock()
	for k, v := range identifiers {
		purgeTime := time.Now().Add(-PurgeInterval)
		if v.Before(purgeTime) {
			delete(identifiers, k)
		}
	}
	mu.Unlock()

	time.AfterFunc(time.Second*60, purgeUsers)

	//	fmt.Printf("Purged users:%d", UserCount())
}
