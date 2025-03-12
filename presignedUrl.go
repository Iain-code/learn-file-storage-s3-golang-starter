package main

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/pingcap/log"
)

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {

	ctx := context.Background()
	time := s3.WithPresignExpires(expireTime)

	// The presign client has methods specifically for generating presigned URLs.
	presignClient := s3.NewPresignClient(s3Client)

	getObject := &s3.GetObjectInput{}
	getObject.Bucket = &bucket
	getObject.Key = &key

	// This generates a presigned URL for getting the specified object.
	// The URL includes the necessary authentication parameters to access the private object
	// PresignGetObject method is doing quite a lot behind the scenes:

	// It adds authentication parameters to this URL as query parameters, including:
	//	X-Amz-Algorithm: The signing algorithm used (typically AWS4-HMAC-SHA256)
	//	X-Amz-Credential: Your AWS access key and scope information
	//	X-Amz-Date: The current date/time
	//	X-Amz-Expires: How long the signature is valid for
	//	X-Amz-SignedHeaders: Which headers are included in the signature
	//	X-Amz-Signature: The actual cryptographic signature
	presignedRequest, err := presignClient.PresignGetObject(ctx, getObject, time)
	if err != nil {
		return "", err
	}

	return presignedRequest.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {

	url := *video.VideoURL
	splitStr := strings.Split(url, ",")

	if len(splitStr) < 2 {
		log.Error("wrong input length")
		return video, nil
	}
	bucket := splitStr[0]
	key := splitStr[1]
	time := 1 * time.Hour

	presignedUrl, err := generatePresignedURL(cfg.s3Client, bucket, key, time)
	if err != nil {
		log.Error("failed to presign url")
		return video, err
	}
	video.VideoURL = &presignedUrl

	return video, nil
}
