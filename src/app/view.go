package app

import (
	"os"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/assets"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/helpers"
)

// SetupAssets compiles or copies our assets from src into the public assets folder.
func SetupAssets() {
	defer log.Time(time.Now(), log.V{"msg": "Finished loading assets"})

	assetsCompiled := config.GetBool("assets_compiled")

	// Init the pkg global for use in ServeAssets
	appAssets = assets.New(assetsCompiled)

	// Always recompile, in production too: secrets/assets.json and public/
	// are persisted volumes, so a manifest from an older image would lack
	// newly added assets.
	log.Info(log.V{"msg": "Compiling Assets"})
	err := appAssets.Compile("src", "public")
	if err != nil {
		log.Fatal(log.V{"a": "unable to compile assets", "error": err})
		os.Exit(1)
	}

	// Set up helpers which are aware of fingerprinted assets
	// These behave differently depending on the compile flag above
	// when compile is set to no, they use precompiled assets
	// otherwise they serve all files in a group separately
	view.Helpers["style"] = appAssets.StyleLink
	view.Helpers["script"] = appAssets.ScriptLink

}

// SetupView sets up the view package by loadind templates.
func SetupView() {
	defer log.Time(time.Now(), log.V{"msg": "Finished loading templates"})

	view.Helpers["markup"] = helpers.Markup
	view.Helpers["timeago"] = helpers.TimeAgo
	view.Helpers["root_url"] = helpers.RootURL

	view.Production = config.Production()
	err := view.LoadTemplates()
	if err != nil {
		//	server.Fatalf("Error reading templates %s", err)
		log.Fatal(log.V{"msg": "unable to read templates", "error": err})
		os.Exit(1)
	}

}
