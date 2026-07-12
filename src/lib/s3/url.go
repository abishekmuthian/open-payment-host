package s3

import (
	"log"
	"time"

	"github.com/abishekmuthian/open-payment-host/src/lib/server/config"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

// GeneratePresignedUrl generates the url for download from the S3
func GeneratePresignedUrl(s3bucket string, s3key string) (string, error) {

	// Region defaults to us-east-1 (AWS S3 default) when unset
	region := config.Get("s3_region")
	if region == "" {
		region = "us-east-1"
	}

	awsConfig := &aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(config.Get("s3_access_key"), config.Get("s3_secret_key"), ""),
	}

	// When a custom endpoint is set (e.g. Cloudflare R2), point the SDK at it
	// and use path-style addressing, which is the safer default for
	// S3-compatible stores. When empty, the SDK falls back to AWS S3.
	endpoint := config.Get("s3_endpoint")
	if endpoint != "" {
		awsConfig.Endpoint = aws.String(endpoint)
		awsConfig.S3ForcePathStyle = aws.Bool(true)
	}

	sess, err := session.NewSession(awsConfig)

	// Create S3 service client
	svc := s3.New(sess)

	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s3bucket),
		Key:    aws.String(s3key),
	})
	urlStr, err := req.Presign(15 * time.Minute)

	if err != nil {
		log.Println("Failed to sign request", err)
	}

	log.Println("The URL is", urlStr)

	return urlStr, err
}
