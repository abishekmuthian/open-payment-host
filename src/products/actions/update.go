package storyactions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	filehelper "github.com/abishekmuthian/open-payment-host/src/lib/model/file"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"
)

// HandleUpdateShow renders the form to update a story.
func HandleUpdateShow(w http.ResponseWriter, r *http.Request) error {

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	// Authorise update story
	currentUser := session.CurrentUser(w, r)
	err = can.Update(story, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)

	price, err := json.Marshal(story.Price)

	if err == nil && price != nil {
		view.AddKey("price", string(price))
	}

	view.AddKey("story", story)
	view.AddKey("currentUser", currentUser)
	view.AddKey("meta_foot", config.Get("meta_desc"))

	// To add the scripts for update page
	view.AddKey("loadTrixScript", true)

	if fileInfo, err := os.Stat("public/assets/images/products/" + fmt.Sprintf("%d-%s-%s", story.ID, filehelper.SanitizeName(story.Name), "featured_image") + ".png"); errors.Is(err, os.ErrNotExist) {
		// Featured image.jpg does not exist
		log.Error(log.V{"Product Update, Featured image does not exist": err})

		if fileInfo, err = os.Stat("public/assets/images/products/" + fmt.Sprintf("%d-%s-%s", story.ID, filehelper.SanitizeName(story.Name), "featured_image") + ".jpg"); errors.Is(err, os.ErrNotExist) {
			// Featured image.png does not exist
			log.Error(log.V{"Product Update, Featured image does not exist": err})
		} else {
			// Featured image.png exists
			view.AddKey("featuredImagePath", config.Get("root_url")+"/assets/images/products/"+fileInfo.Name())
		}
	} else {
		// Featured image.jpg exists
		view.AddKey("featuredImagePath", config.Get("root_url")+"/assets/images/products/"+fileInfo.Name())
	}

	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())

	return view.Render()
}

// HandleUpdate handles the POST of the form to update a story
func HandleUpdate(w http.ResponseWriter, r *http.Request) error {

	// Fetch the  params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	name := params.Get("name")
	id := params.GetInt("id")

	// Find the story
	story, err := products.Find(params.GetInt(products.KeyName))
	if err != nil {
		return server.NotFoundError(err)
	}

	// Check the authenticity token
	err = session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Authorise update story
	currentUser := session.CurrentUser(w, r)
	err = can.Update(story, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Check if product has more than 2 hashtags
	if CountHashTag(name) > 2 {
		return server.NotAuthorizedError(nil, "Hashtag too many or format error", "Your product has too many hashtags, title should be your product in 50 characters followed by 2 hashtags. You can click back safely to edit what you had typed.")
	}

	// Clean params according to role
	accepted := products.AllowedParams()
	if currentUser.Admin() {
		accepted = products.AllowedParamsAdmin()
	}
	storyParams := story.ValidateParams(params.Map(), accepted)

	err = story.Update(storyParams)
	if err != nil {
		return server.InternalError(err)
	}

	//Update featured image for other than default posts
	// FIXME : Add error handling
	/* 	if id > 5 && (file.SanitizeName(name) != file.SanitizeName(story.Name)) {
		texttoimage.TextToImage(name, id)
	} */

	for _, fh := range params.Files {

		fileType := fh[0].Header.Get("Content-Type")
		fileSize := fh[0].Size

		log.Info(log.V{"Product Submission": "File Upload", "fileType": fileType})
		log.Info(log.V{"Product Submission": "File Upload", "fileSize (kB)": fileSize / 1000})

		if fileType == "image/png" || fileType == "image/jpeg" {
			file, err := fh[0].Open()
			defer file.Close()

			if err != nil {
				log.Error(log.V{"Create Product, Error storing featured image": err})
			}

			fileData, err := ioutil.ReadAll(file)
			if err != nil {
				log.Error(log.V{"Create Product, Error storing featured image": err})
			}

			var fileExtension string

			if fileType == "image/png" {
				fileExtension = ".png"
			}

			if fileType == "image/jpeg" {
				fileExtension = ".jpg"
			}

			outFile, err := os.Create("public/assets/images/products/" + fmt.Sprintf("%d-%s-%s", id, filehelper.SanitizeName(name), "featured_image") + fileExtension)
			if err != nil {
				log.Error(log.V{"msg": "Image creation, Creating empty file", "error": err})
				os.Exit(1)
			}
			defer outFile.Close()

			outFile.Write(fileData)
		} else {

		}

	}

	// Redirect to story
	return server.Redirect(w, r, story.ShowURL())
}
