package main

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"

	"github.com/abishekmuthian/open-payment-host/src/app"
)

// Main entrypoint for the server which performs bootstrap, setup
// then runs the server. Most setup is delegated to the src/app pkg.
func main() {

	mu := &sync.RWMutex{}

	// Setup our server
	server, err := SetupServer(mu)
	if err != nil {
		fmt.Printf("server: error setting up %s\n", err)
		return
	}

	// Inform user of server setup
	server.Logf("#info Starting server in %s mode on port %d", server.Mode(), server.Port())

	// In production, server
	if server.Production() {

		// Redirect all :80 traffic to our canonical url on :443
		//server.StartRedirectAll(80, server.Config("root_url"))

		// If in production, serve over tls with autocerts from let's encrypt
		err = server.StartTLSAuto(server.Config("autocert_email"), server.Config("autocert_domains"))
		if err != nil {
			server.Fatalf("Error starting server %s", err)
		}

	} else {
		// In development just serve with http on local port
		err := server.Start()
		if err != nil {
			server.Fatalf("Error starting server %s", err)
		}
	}

}

// SetupServer creates a new server, and delegates setup to the app pkg.
func SetupServer(mu *sync.RWMutex) (*server.Server, error) {

	// Setup server
	s, err := server.New()
	if err != nil {
		return nil, err
	}

	// Load the appropriate config
	c := config.New()
	err = c.Load("secrets/fragmenta.json")
	if err != nil {
		log.Fatal(err)
	}
	config.Current = c

	// Check environment variable to see if we are in production mode
	if os.Getenv("FRAG_ENV") == "production" {
		config.Current.Mode = config.ModeProduction
	}

	// Call the app to perform additional setup
	app.Setup(mu)

	return s, nil
}
