package file

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"

	// We support gif for reading, but not writing
	_ "image/gif"

	// TODO: vendor rez
	"github.com/bamiaux/rez"
)

// Options represents image options
type Options struct {
	Path      string
	MaxHeight int64
	MaxWidth  int64
	Quality   int
	// Square bool
}

// SaveJpegRepresentations saves several image representation rescaled in proportion (always in proportion) using the specified max width, max height and quality
func SaveJpegRepresentations(r io.Reader, options []Options) error {

	if r == nil {
		return errors.New("Nil reader received in SaveJpegRepresentation")
	}

	// Read the image data, if we have a jpeg, convert to png?
	original, _, err := image.Decode(r)
	if err != nil {
		return err
	}

	// For each option, save a file
	for _, o := range options {

		//fmt.Printf("Saving image file - %v\n", o)

		// Resize this image given the params - this is always in proportion, NEVER stretched
		// If Square is true we crop to a square
		resized, err := ResizeImage(original, o.MaxWidth, o.MaxHeight, false)
		if err != nil {
			return err
		}

		// Write out to the desired file path
		w, err := os.Create(o.Path)
		if err != nil {
			return err
		}
		defer w.Close()
		err = jpeg.Encode(w, resized, &jpeg.Options{Quality: o.Quality})
		if err != nil {
			return err
		}

	}

	return nil

}

// SavePNGRepresentations saves png representations according to options
func SavePNGRepresentations(r io.Reader, options []Options) error {

	if r == nil {
		return errors.New("Nil reader received in SaveJpegRepresentation")
	}

	// Read the image data, if we have a jpeg, convert to png?
	original, _, err := image.Decode(r)
	if err != nil {
		return err
	}

	// For each option, save a file
	for _, o := range options {

		fmt.Printf("Saving image file - %v\n", o)

		// Resize this image given the params - this is always in proportion, NEVER stretched
		// If Square is true we crop to a square
		resized, err := ResizeImage(original, o.MaxWidth, o.MaxHeight, false)
		if err != nil {
			return err
		}

		// Write out to the desired file path
		w, err := os.Create(o.Path)
		if err != nil {
			return err
		}
		defer w.Close()
		err = png.Encode(w, resized)
		if err != nil {
			return err
		}

	}

	return nil

}

// ResizeImage resizes the given image IF it is larger than maxWidth or maxHeight
func ResizeImage(src image.Image, maxWidth int64, maxHeight int64, square bool) (image.Image, error) {
	var dst image.Image

	// Check the original dimensions first, and bail out if this image is not larger than max dimensions
	srcSize := src.Bounds().Size()
	if int64(srcSize.X) < maxWidth && int64(srcSize.Y) < maxHeight {
		return src, nil
	}

	// Use the original image dimensions to keep it in pro
	// Distorting images is a sin of which we are never guilty
	ratio := float64(maxWidth) / float64(srcSize.X)
	yRatio := float64(maxHeight) / float64(srcSize.Y)
	if yRatio < ratio {
		ratio = yRatio
	}

	// Now adjust desired width and height according to ratio
	width := float64(srcSize.X) * ratio
	height := float64(srcSize.Y) * ratio

	// Create a new resized image with the desired dimensions and fill it with resized image data
	// We switch on input image type - is YCbCrSubsampleRatio444 correct?
	switch src.(type) {
	case *image.YCbCr:
		dst = image.NewYCbCr(image.Rect(0, 0, int(width), int(height)), image.YCbCrSubsampleRatio444)
	case *image.RGBA:
		dst = image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	default:
		dst = nil
	}

	err := rez.Convert(dst, src, rez.NewBicubicFilter())
	// IF we want thumbnails to be square/cropped we could do this
	// for now we don't need this. We may not even want it for camping?
	//   err :=  imaging.Thumbnail(srcImage, 100, 100, imaging.Lanczos)

	if err != nil {
		return nil, err
	}

	return dst, nil

}
