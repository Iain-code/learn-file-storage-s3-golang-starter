package main

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	const maxMemory = 1 << 30
	http.MaxBytesReader(w, r.Body, maxMemory)

	videoIDString := r.PathValue("videoID")

	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	user, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Video not found", err)
		return
	}
	if video.UserID != user {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Video not found", err)
		return
	}
	defer file.Close()

	contentType := fileHeader.Header.Get("Content-Type")
	// Gives the raw content type string from the form data,
	// which might look like: ("video/mp4; charset=utf-8")
	//  or similar, possibly with additional parameters.

	typ, _, err := mime.ParseMediaType(contentType)
	// this then breaks down that string into its components such as "video/mp4"
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}
	if typ != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}
	random := make([]byte, 32)
	_, err = rand.Read(random)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get video", err)
		return
	}

	randomKey := base64.RawURLEncoding.EncodeToString(random)
	randomKey += ".mp4"

	tempFile.Seek(0, io.SeekStart) // Reset the tempFile's file pointer to the beginning
	obj := &s3.PutObjectInput{}
	obj.Bucket = &cfg.s3Bucket
	obj.Key = &randomKey
	obj.Body = tempFile
	obj.ContentType = &contentType

	cfg.s3Client.PutObject(r.Context(), obj)
	url := "https://" + cfg.s3Bucket + ".s3." + cfg.s3Region + ".amazonaws.com/" + *obj.Key
	video.VideoURL = &url
	video.UpdatedAt = time.Now()

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}
}
