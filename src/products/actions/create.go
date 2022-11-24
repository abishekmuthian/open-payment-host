package storyactions

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"

	"github.com/abishekmuthian/open-payment-host/src/lib/auth/can"
	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/view"

	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/abishekmuthian/open-payment-host/src/products"

	filehelper "github.com/abishekmuthian/open-payment-host/src/lib/model/file"
)

// HandleCreateShow serves the create form via GET for products.
func HandleCreateShow(w http.ResponseWriter, r *http.Request) error {

	story := products.New()

	// Authorise
	currentUser := session.CurrentUser(w, r)
	err := can.Create(story, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Render the template
	view := view.NewRenderer(w, r)
	view.AddKey("story", story)
	// To add the scripts for add product page
	view.AddKey("loadTrixScript", true)
	view.AddKey("currentUser", currentUser)
	view.AddKey("meta_foot", config.Get("meta_desc"))
	// Set the name and year
	view.AddKey("name", config.Get("name"))
	view.AddKey("year", time.Now().Year())
	return view.Render()
}

// HandleCreate handles the POST of the create form for products
func HandleCreate(w http.ResponseWriter, r *http.Request) error {

	story := products.New()

	// Check the authenticity token
	err := session.CheckAuthenticity(w, r)
	if err != nil {
		return err
	}

	// Get user details
	currentUser := session.CurrentUser(w, r)

	// Authorise
	err = can.Create(story, currentUser)
	if err != nil {
		return server.NotAuthorizedError(err)
	}

	// Get the params
	params, err := mux.Params(r)
	if err != nil {
		return server.InternalError(err)
	}

	name := params.Get("name")

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

	// Set a few params to known good values
	storyParams["points"] = "1"
	storyParams["user_id"] = fmt.Sprintf("%d", currentUser.ID)
	storyParams["user_name"] = currentUser.Name

	ID, err := story.Create(storyParams)
	if err != nil {
		return err // Create returns a router.Error
	}

	// Log creation
	log.Info(log.V{"msg": "Created story", "story_id": ID})

	// Redirect to the new story
	story, err = products.Find(ID)
	if err != nil {
		return server.InternalError(err)
	}

	// We need to add a vote to the story here too by adding a join to the new id
	// No votes in Open Payment Host yet, POWER in query needs fixing
	/* 	err = recordStoryVote(story, currentUser, ip, +1)
	   	if err != nil {
	   		return err
	   	} */

	// Re-rank products
	// No ranking in Open Payment Host yet, POWER in query needs fixing
	/* 	err = updateproductsRank()
	   	if err != nil {
	   		return err
	   	} */

	//Create featured image for other than default posts
	// Call this when featured image is not set
	// FIXME : Add error handling
	/* 	if ID > 5 {
		texttoimage.TextToImage(name, ID)
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

			outFile, err := os.Create("public/assets/images/products/" + fmt.Sprintf("%d-%s-%s", ID, filehelper.SanitizeName(name), "featured_image") + fileExtension)
			if err != nil {
				log.Error(log.V{"msg": "Image creation, Creating empty file", "error": err})
				os.Exit(1)
			}
			defer outFile.Close()

			outFile.Write(fileData)

		} else {

		}

	}

	return server.Redirect(w, r, story.IndexURL())
}

// RemoveHashTag removes hashtag from the string and returns the string
func RemoveHashTag(name string) string {
	var myregex = `#\w*`
	var re = regexp.MustCompile(myregex)

	var fulltext = re.ReplaceAllString(name, "")
	return fulltext
}

// CountHashTag returns the number of hashtag in the string
func CountHashTag(name string) int {
	var myregex = `#\w*`
	var re = regexp.MustCompile(myregex)

	var fulltext = re.FindAllString(name, -1)
	return len(fulltext)
}
