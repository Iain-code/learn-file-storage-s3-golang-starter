package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/ydb-platform/ydb-go-sdk/v3/log"
)

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {

	ctx := context.Background()
	time := s3.WithPresignExpires(expireTime)

	presignClient := s3.NewPresignClient(s3Client)
	getObject := &s3.GetObjectInput{}
	getObject.Bucket = &bucket
	getObject.Key = &key
	presignedRequest, err := presignClient.PresignGetObject(ctx, getObject, time)
	if err != nil {
		return "", err
	}
	fmt.Printf("presignedRequest.URL: %v\n", presignedRequest.URL)
	return presignedRequest.URL, nil
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {

	url := *video.VideoURL
	splitStr := strings.Split(url, ",")
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
