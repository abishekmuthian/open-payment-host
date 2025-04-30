package storyactions

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/abishekmuthian/open-payment-host/src/lib/mux"
	"github.com/abishekmuthian/open-payment-host/src/lib/server"
	"github.com/abishekmuthian/open-payment-host/src/lib/server/log"
	"github.com/abishekmuthian/open-payment-host/src/lib/session"
	"github.com/google/uuid"
)

// HandleFileAttachment handles file upload for the post from the editor
func HandleFileAttachment(w http.ResponseWriter, r *http.Request) error {
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

	// File Attachment
	for _, fh := range params.Files {

		fileType := fh[0].Header.Get("Content-Type")
		fileSize := fh[0].Size
		filename := fh[0].Header.Get("Filename")

		log.Info(log.V{"Product Submission": "Attachment File Upload", "fileType": fileType})
		log.Info(log.V{"Product Submission": "Attachment File Upload", "fileSize (kB)": fileSize / 1000})

		if fileType == "image/png" || fileType == "image/jpeg" || fileType == "image/gif" || fileType == "video/mp4" {

			file, err := fh[0].Open()
			defer file.Close()

			if err != nil {
				log.Error(log.V{"Create Product, Error storing attachment file": err})
			}

			fileData, err := io.ReadAll(file)
			if err != nil {
				log.Error(log.V{"Create Product, Error storing attachment file": err})
			}

			ext := filepath.Ext(filename)
			newFileName := fmt.Sprintf("%s%s", uuid.New().String(), ext)

			var fileExtension string

			if fileType == "image/png" {
				fileExtension = ".png"
			}

			if fileType == "image/jpeg" {
				fileExtension = ".jpg"
			}

			if fileType == "image/gif" {
				fileExtension = ".gif"
			}

			if fileType == "video/mp4" {
				fileExtension = ".mp4"
			}

			outFile, err := os.Create("data/public/assets/images/products/" + newFileName + fileExtension)
			if err != nil {
				log.Error(log.V{"msg": "File creation, Creating empty file", "error": err})
			} else {

				type FileAttachmentResponse struct {
					URL string `json:"url"`
				}

				fileAttachmentResponse := FileAttachmentResponse{
					URL: "/assets/images/products/" + newFileName + fileExtension,
				}

				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(fileAttachmentResponse)
			}

			defer outFile.Close()

			outFile.Write(fileData)

		} else {
			// TODO wrong image format inform user
			return server.InternalError(errors.New("Improper image format only png, jpg or gif image format is allowed."))
		}

	}

	return err

}
