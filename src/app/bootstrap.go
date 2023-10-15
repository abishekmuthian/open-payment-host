package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"sync"
)

// TODO: Most of this should probably go into a config/bootstrap package within fragmenta?
//	"github.com/abishekmuthian/iwillpayforthat/src/lib/server/fragmenta/config"

const (
	version                     = "0.0.1"
	permissions                 = 0744
	createDatabaseMigrationName = "Create-Database"
	createTablesMigrationName   = "Create-Tables"
)

var (
	// ConfigDevelopment holds the development config from fragmenta.json
	ConfigDevelopment map[string]string

	// ConfigProduction holds development config from fragmenta.json
	ConfigProduction map[string]string

	// ConfigTest holds the app test config from fragmenta.json
	ConfigTest map[string]string
)

// Bootstrap generates missing config files, sql migrations, and runs the first migrations
// For this we need to know what to call the app, but we default to abishek/iwillpayforthat for now
// we could use our current folder name?
func Bootstrap(mu *sync.RWMutex) error {
	// We assume we're being run from root of project path
	projectPath, err := os.Getwd()
	if err != nil {
		return err
	}

	fmt.Printf("\nBootstrapping server...\n")

	err = generateConfig(projectPath)
	if err != nil {
		return err
	}

	return nil
}

// RequiresBootStrap returns true if the app requires bootstrapping
func RequiresBootStrap() bool {
	if !fileExists(configPath()) {
		return true
	}
	return false
}

func configPath() string {
	return "secrets/fragmenta.json"
}

func generateConfig(projectPath string) error {
	configPath := configPath()
	log.Printf("Generating new config at %s", configPath)

	ConfigProduction = map[string]string{}
	ConfigDevelopment = map[string]string{
		"assets_compiled":             "no",
		"db":                          "./db/oph.db?_journal_mode=WAL",
		"db_adapter":                  "sqlite3",
		"hmac_key":                    randomKey(32),
		"secret_key":                  randomKey(32),
		"log":                         "log/development.log",
		"name":                        "Open Payment Host",
		"meta_url":                    "",
		"meta_title":                  "Sell what you want without paying commissions",
		"meta_desc":                   "Sell Subscriptions, Newsletters, Digital Files without paying commissions.",
		"meta_keywords":               "payments,subscription,projects,products",
		"meta_image":                  "/assets/images/app/oph_featured_image.png",
		"port":                        "3000",
		"domain":                      "localhost",
		"root_url":                    "http://localhost:3000",
		"session_name":                "open_payment_host_session",
		"admin_email":                 "admin@openpaymenthost.com",
		"admin_default_password":      "OpenPaymentHost",
		"reset_admin":                 "no",
		"turnstile_site_key":          "1x00000000000000000000AA",
		"turnstile_secret_key":        "1x0000000000000000000000000000000AA",
		"mailchimp_token":             "",
		"stripe_key":                  "",
		"stripe_secret":               "",
		"stripe_webhook_secret":       "",
		"stripe_callback_domain":      "",
		"stripe_tax_rate_IN":          "",
		"subscription_client_country": "US",
		"square_access_token":         "",
		"square_app_id":               "",
		"square_location_id":          "",
		"square_notification_url":     "",
		"square_signature_key":        "",
		"stripe":                      "no",
		"square":                      "yes",
		"square_domain":               "https://connect.squareupsandbox.com/v2",
		"square_sandbox_source_id":    "cnon:card-nonce-ok",
		"s3_access_key":               "",
		"s3_secret_key":               "",
		"palm_key":                    "",
	}

	// Copying development values to production and then adding more
	for k, v := range ConfigDevelopment {
		ConfigDevelopment[k] = v
		ConfigProduction[k] = v
	}

	ConfigProduction["assets_compiled"] = "no"
	ConfigProduction["log"] = "log/production.log"
	ConfigProduction["port"] = "443"
	ConfigProduction["assets_compiled"] = "yes"
	ConfigProduction["autocert_email"] = ""
	ConfigProduction["autocert_domains"] = ""
	ConfigProduction["hmac_key"] = randomKey(32)
	ConfigProduction["secret_key"] = randomKey(32)
	ConfigProduction["turnstile_site_key"] = ""
	ConfigProduction["turnstile_secret_key"] = ""
	ConfigProduction["root_url"] = ""
	ConfigProduction["square_domain"] = "https://connect.squareup.com/v2"

	configs := map[string]map[string]string{
		"production":  ConfigProduction,
		"development": ConfigDevelopment,
	}

	configJSON, err := json.MarshalIndent(configs, "", "\t")
	if err != nil {
		log.Printf("Error parsing config %s %v", configPath, err)
		return err
	}

	// Write the config json file
	err = ioutil.WriteFile(configPath, configJSON, permissions)
	if err != nil {
		log.Printf("Error writing config %s %v", configPath, err)
		return err
	}

	return nil
}

// Generate a random 32 byte key encoded in base64
func randomKey(l int64) string {
	k := make([]byte, l)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		return ""
	}
	return hex.EncodeToString(k)
}

// fileExists returns true if this file exists
func fileExists(p string) bool {
	_, err := os.Stat(p)
	if err != nil && os.IsNotExist(err) {
		return false
	}

	return true
}
