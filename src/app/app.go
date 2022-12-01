package app

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/assets"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/users"
	useractions "github.com/abishekmuthian/open-payment-host/src/users/actions"

	"github.com/abishekmuthian/open-payment-host/src/lib/mail"
	"github.com/abishekmuthian/open-payment-host/src/lib/mail/adapters/sendgrid"
)

// appAssets holds a reference to our assets for use in asset setup
var appAssets *assets.Collection

// Setup sets up our application
func Setup(mu *sync.RWMutex) {

	// Setup log
	err := SetupLog()
	if err != nil {
		println("failed to set up logs %s", err)
		os.Exit(1)
	}

	// Log server startup
	msg := "Starting server"
	if config.Production() {
		msg = msg + " in production"
	}

	log.Info(log.Values{"msg": msg, "port": config.Get("port")})
	defer log.Time(time.Now(), log.Values{"msg": "Finished loading server"})

	// Set up scheduling service interfaces
	//SetupServices()

	// Set up our assets
	SetupAssets()

	// Setup our view templates
	SetupView()

	// Setup our database
	SetupDatabase(mu)

	// Setup our authentication and authorisation
	SetupAuth()

	// Setup our router and handlers
	SetupRoutes()

	// Setup mail from config
	// SetupMail()

	// Set up default user
	SetupDefaultUser()

}

// SetupLog sets up logging
func SetupLog() error {

	// Set up a stderr logger with time prefix
	logger, err := log.NewStdErr(log.PrefixDateTime)
	if err != nil {
		return err
	}
	log.Add(logger)

	// Set up a file logger pointing at the right location for this config.
	fileLog, err := log.NewFile(config.Get("log"))
	if err != nil {
		return err
	}
	log.Add(fileLog)

	return nil
}

// SetupDefaultUser creates the default user for the application if necessary
func SetupDefaultUser() {
	_, err := users.Find(1)
	if err != nil {
		log.Error(log.V{"Setup App, Setup Default User": err})

		if strings.Contains(err.Error(), "No results found") {
			log.Info(log.V{"App": "Creating admin user..."})
			useractions.HandleCreate(config.Get("admin_email"), config.Get("admin_default_password"))
			if err != nil {
				log.Error(log.V{"App, Error creating admin user": err})
			}
		}
	} else {
		log.Info(log.V{"Setup, App": "Default user already exists"})
	}

	if config.Get("reset_admin") == "true" {
		log.Info(log.V{"App": "Resetting the email and password of the admin to default credentials"})
		useractions.HandleUpdate(1, config.Get("admin_email"), config.Get("admin_default_password"))
	}

}

// SetupMail sets us up to send mail via mailchimp (requires key).
func SetupMail() {
	mail.Production = config.Production()
	mail.Service = sendgrid.New(config.Get("mail_from"), config.Get("mail_secret"))
}
