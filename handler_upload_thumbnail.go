package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
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

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)

	file, handler, err := r.FormFile("thumbnail")
	// Retrieves a file that was uploaded in an HTML form.
	// It extracts a file from a **MULTIPART** form submission where the form field name is "thumbnail"
	//
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	content := handler.Header.Get("Content-Type") // searches headers for content type (in this case image)
	typ, _, err := mime.ParseMediaType(content)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}
	if typ != "image/png" && typ != "image/jpeg" {
		respondWithError(w, http.StatusBadRequest, "Invalid Request", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID) // finds video using video.ID
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get video", err)
		return
	}
	if userID != video.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized", err)
		return
	}
	random := make([]byte, 32)
	_, err = rand.Read(random)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get video", err)
		return
	}
	filename := base64.RawURLEncoding.EncodeToString(random)

	split := strings.Split(content, "/")
	root := "./" + cfg.assetsRoot
	pth := filepath.Join(root, filename)
	path := pth + "." + split[1]
	newFile, err := os.Create(path)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Reqeust", err)
		return
	}

	_, err = io.Copy(newFile, file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Reqeust", err)
		return
	}

	host := "http://localhost:8091/app/"
	url := host + path
	video.ThumbnailURL = &url

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
