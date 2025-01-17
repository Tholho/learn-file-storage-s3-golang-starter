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

	// TODO: implement the upload here
	const maxMemory = 10 << 20

	r.ParseMultipartForm(maxMemory)
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		fmt.Println(err)
	}
	mediaType := header.Header.Get("Content-Type")
	fmt.Println("Media type is", mediaType)
	byteData, err := io.ReadAll(file)
	if err != nil {
		fmt.Println(err)
	}
	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		fmt.Println(err)
	}
	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Video does not belong to User", err)
	}
	thumbnail := thumbnail{}
	mimeType, params, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldnt parse mime type", err)
		fmt.Println("Invalid mime type:", mimeType, params)
	}
	if mimeType != "image/jpeg" && mimeType != "image/png" {
		respondWithError(w, http.StatusExpectationFailed, "Invalid file type, expected .jpeg or .png", err)
		fmt.Println("User tried to upload file of type:", mimeType, "with parameters", params)
		return
	}
	thumbnail.mediaType = mediaType
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		respondWithError(w, http.StatusInternalServerError, "Invalid media type", err)
		fmt.Println("Invalid media type:", mediaType)
		return
	}
	extension := parts[1]
	thumbnail.data = byteData
	//videoThumbnails[videoID] = thumbnail
	randomArr := make([]byte, 32)
	_, err = rand.Read(randomArr)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Filename error", err)
		fmt.Println(err)
		return
	}
	randomName := base64.RawURLEncoding.EncodeToString(randomArr)
	videoWithExt := randomName + "." + extension
	videoPath := filepath.Join(cfg.assetsRoot, videoWithExt)
	videoFile, err := os.Create(videoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error creating file", err)
		fmt.Println(err)
		return
	}
	defer videoFile.Close()
	if seeker, ok := file.(io.Seeker); ok {
		_, err = seeker.Seek(0, io.SeekStart)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "Error seeking file", err)
			return
		}
	}
	_, err = io.Copy(videoFile, file)
	if err != nil {
		fmt.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Error copying file", err)
		return
	}
	relativePath := filepath.Base(videoPath)
	//http://localhost:8091./assets1ab5f922-9f3f-479f-8020-1583e03265da.png
	thumbnailURL := "http://localhost:" + cfg.port + "/assets/" + relativePath
	fmt.Println(thumbnailURL)
	//thumbnailURL := "http://localhost:8091/assets/" + videoID.String() + "." + thumbnail.mediaType
	videoData.ThumbnailURL = &thumbnailURL
	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		fmt.Println(err)
	}
	respondWithJSON(w, http.StatusOK, videoData)
}
