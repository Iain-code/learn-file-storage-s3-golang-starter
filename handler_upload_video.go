package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	// Uploads video to AWS with s3 client

	// max memory set for incoming request
	const maxMemory = 1 << 30
	http.MaxBytesReader(w, r.Body, maxMemory)

	// extracts the videoID part from the URL path given
	videoIDString := r.PathValue("videoID")

	//  parses the string into a UUID so can be used with funcs as an ID number
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	// gets the token from the header as a string
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}
	// uses token string from header and Secret string in cfg to validate token/user
	user, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	// gets video from video ID query from database
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Video not found", err)
		return
	}
	if video.UserID != user {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	// takes keyword "video" given by front end HTML...possibly coded on a button on the site or something similar
	file, fileHeader, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Video not found", err)
		return
	}
	// close file to stop leakage
	defer file.Close()

	// Gives the raw content type string from the form data,
	// which might look like: ("video/mp4; charset=utf-8")
	//  or similar, possibly with additional parameters.
	contentType := fileHeader.Header.Get("Content-Type")

	// this then breaks down that string into its components such as "video/mp4"
	typ, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}
	if typ != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}
	// creates tempory file to save the data in while we move it from request into s3/AWS
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}

	// copy the file from the request over to the temp file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}

	// make "key" or "name" for file that will be saved in s3 so it can be re-called
	random := make([]byte, 32)
	_, err = rand.Read(random)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get video", err)
		return
	}
	// "parse" / "encode" key to string
	randomKey := base64.RawURLEncoding.EncodeToString(random)
	randomKey += ".mp4"

	// Reset the tempFile's file pointer to the start of the file.
	// the pointer is telling all functions or actions where to START READING the file from
	// after copying the request.Body file to tempFile, the pointer will now be pointing at the end of tempFile.
	// if we then tried to copy to s3, there would be nothing to copy coz of where the pointer is pointing.
	tempFile.Seek(0, io.SeekStart)

	filePath := tempFile.Name()
	fmt.Printf("filepath 1: %v\n", filePath)
	aspectRatio, err := getVideoAspectRatio(filePath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}

	var prefix string
	if aspectRatio == "16:9" {
		prefix = "landscape/"
	} else if aspectRatio == "9:16" {
		prefix = "portrait/"
	} else {
		prefix = "other/"
	}

	processedFilePath, err := processVideoForFastStart(filePath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to process video", err)
		return
	}

	processedFile, err := os.Open(processedFilePath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "failed to open file", err)
		return
	}

	// when defers are called its Last In First Out (LIFO) so close will be called before remove
	defer processedFile.Close()
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	tempFile.Seek(0, io.SeekStart)

	key := prefix + randomKey

	url := cfg.cloudFront + key

	// s3 client / upload struct for upload to AWS s3
	obj := &s3.PutObjectInput{}
	obj.Bucket = &cfg.s3Bucket // name of bucket used
	obj.Key = &key             // key used to identify file on s3 db
	obj.Body = processedFile
	obj.ContentType = &contentType

	// PutObject actually adds the object (obj.Body = tempFile -> now processedFile) with other metadata to s3 BUCKET
	_, err = cfg.s3Client.PutObject(r.Context(), obj)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to PutObject", err)
		return
	}
	video.VideoURL = &url
	video.UpdatedAt = time.Now()
	fmt.Printf("videoURL: %v", video.VideoURL)

	err = cfg.db.UpdateVideo(video) // update video in sqlite3
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}
}

func getVideoAspectRatio(filePath string) (string, error) {

	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var b bytes.Buffer
	cmd.Stdout = &b
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	type VideoStats struct {
		Streams []struct {
			Height int `json:"height"`
			Width  int `json:"width"`
		} `json:"streams"`
	}
	videoStats := &VideoStats{}

	err = json.Unmarshal(b.Bytes(), videoStats)
	if err != nil {
		return "", err
	}

	width := videoStats.Streams[0].Width
	height := videoStats.Streams[0].Height

	if width == 16*height/9 {
		return "16:9", nil
	} else if height == 16*width/9 {
		return "9:16", nil
	}
	return "other", nil

}

func processVideoForFastStart(filePath string) (string, error) {

	rtnPath := filePath + ".processing"

	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", rtnPath)

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return rtnPath, nil
}
