package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

type streamsArr struct {
	Streams []struct {
		Index              int    `json:"index"`
		CodecName          string `json:"codec_name,omitempty"`
		CodecLongName      string `json:"codec_long_name,omitempty"`
		Profile            string `json:"profile,omitempty"`
		CodecType          string `json:"codec_type"`
		CodecTagString     string `json:"codec_tag_string"`
		CodecTag           string `json:"codec_tag"`
		Width              int    `json:"width,omitempty"`
		Height             int    `json:"height,omitempty"`
		CodedWidth         int    `json:"coded_width,omitempty"`
		CodedHeight        int    `json:"coded_height,omitempty"`
		ClosedCaptions     int    `json:"closed_captions,omitempty"`
		HasBFrames         int    `json:"has_b_frames,omitempty"`
		SampleAspectRatio  string `json:"sample_aspect_ratio,omitempty"`
		DisplayAspectRatio string `json:"display_aspect_ratio,omitempty"`
		PixFmt             string `json:"pix_fmt,omitempty"`
		Level              int    `json:"level,omitempty"`
		ColorRange         string `json:"color_range,omitempty"`
		ColorSpace         string `json:"color_space,omitempty"`
		ColorTransfer      string `json:"color_transfer,omitempty"`
		ColorPrimaries     string `json:"color_primaries,omitempty"`
		ChromaLocation     string `json:"chroma_location,omitempty"`
		Refs               int    `json:"refs,omitempty"`
		IsAvc              string `json:"is_avc,omitempty"`
		NalLengthSize      string `json:"nal_length_size,omitempty"`
		RFrameRate         string `json:"r_frame_rate"`
		AvgFrameRate       string `json:"avg_frame_rate"`
		TimeBase           string `json:"time_base"`
		StartPts           int    `json:"start_pts"`
		StartTime          string `json:"start_time"`
		DurationTs         int    `json:"duration_ts"`
		Duration           string `json:"duration"`
		BitRate            string `json:"bit_rate,omitempty"`
		BitsPerRawSample   string `json:"bits_per_raw_sample,omitempty"`
		NbFrames           string `json:"nb_frames"`
		Disposition        struct {
			Default         int `json:"default"`
			Dub             int `json:"dub"`
			Original        int `json:"original"`
			Comment         int `json:"comment"`
			Lyrics          int `json:"lyrics"`
			Karaoke         int `json:"karaoke"`
			Forced          int `json:"forced"`
			HearingImpaired int `json:"hearing_impaired"`
			VisualImpaired  int `json:"visual_impaired"`
			CleanEffects    int `json:"clean_effects"`
			AttachedPic     int `json:"attached_pic"`
			TimedThumbnails int `json:"timed_thumbnails"`
		} `json:"disposition"`
		Tags struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			VendorID    string `json:"vendor_id"`
			Encoder     string `json:"encoder"`
			Timecode    string `json:"timecode"`
		} `json:"tags,omitempty"`
		SampleFmt     string `json:"sample_fmt,omitempty"`
		SampleRate    string `json:"sample_rate,omitempty"`
		Channels      int    `json:"channels,omitempty"`
		ChannelLayout string `json:"channel_layout,omitempty"`
		BitsPerSample int    `json:"bits_per_sample,omitempty"`
		Tags0         struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			VendorID    string `json:"vendor_id"`
		} `json:"tags,omitempty"`
		Tags1 struct {
			Language    string `json:"language"`
			HandlerName string `json:"handler_name"`
			Timecode    string `json:"timecode"`
		} `json:"tags,omitempty"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmdPtr := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var bfr bytes.Buffer
	cmdPtr.Stdout = &bfr
	err := cmdPtr.Run()
	if err != nil {
		fmt.Println(err)
	}
	//fmt.Println(bfr.String())
	streamsArrStorage := streamsArr{}
	err = json.Unmarshal(bfr.Bytes(), &streamsArrStorage)
	//fmt.Println(streamsArrStorage)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	ratio := float64(streamsArrStorage.Streams[0].Width) / float64(streamsArrStorage.Streams[0].Height)
	ref1 := 16.0 / 9.0
	ref2 := 9.0 / 16.0
	if ratio > ref1-0.1 && ratio < ref1+0.1 {
		return "16:9", nil
	} else if ratio > ref2-0.1 && ratio < ref2+0.1 {
		return "9:16", nil
	}
	return "other", nil
}

func processVideoForFastStart(filePath string) (string, error) {
	newPath := strings.Split(filePath, ".mp4")[0] + ".processing.mp4"
	cmdPtr := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", newPath)
	err := cmdPtr.Run()
	if err != nil {
		fmt.Println(err)
	}
	return newPath, nil
}

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	uploadLimit := 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, int64(uploadLimit))
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
	videoData, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldnt retrieve videoID", err)
		fmt.Println(err)
		return
	}
	if videoData.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Video does not belong to User", err)
	}
	file, header, err := r.FormFile("video")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()
	mediaType := header.Header.Get("Content-Type")
	fmt.Println("Media type is", mediaType)
	mimeType, params, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldnt parse mime type", err)
		fmt.Println("Invalid mime type:", mimeType, params)
		return
	}
	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusExpectationFailed, "Invalid file type, expected mp4", err)
		fmt.Println("User tried to upload file of type:", mimeType, "with parameters", params)
		return
	}
	tmpFile, err := os.CreateTemp("", "tubely-upload-*.mp4")
	if err != nil {
		fmt.Println(err)
		return
	}
	//defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	_, err = io.Copy(tmpFile, file)
	if err != nil {
		fmt.Println(err)
		return
	}
	ratio, err := getVideoAspectRatio(tmpFile.Name())
	aspect := ""
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "VideoAspectRatio error", err)
		fmt.Println(err)
		return
	}
	if ratio == "16:9" {
		aspect = "landscape/"
	} else if ratio == "9:16" {
		aspect = "portrait/"
	} else {
		aspect = "other/"
	}
	_, err = tmpFile.Seek(0, io.SeekStart)
	if err != nil {
		fmt.Println(err)
		return
	}
	processedVideoPath, err := processVideoForFastStart(tmpFile.Name())
	if err != nil {
		fmt.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Processing video error", err)
		return
	}
	processedVideo, err := os.Open(processedVideoPath)
	if err != nil {
		fmt.Println(err)
		respondWithError(w, http.StatusInternalServerError, "Processed video cannot be opened", err)
		return
	}
	defer processedVideo.Close()
	putObjectParams := s3.PutObjectInput{}
	putObjectParams.Bucket = &cfg.s3Bucket
	randomArr := make([]byte, 32)
	_, err = rand.Read(randomArr)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Filename error", err)
		fmt.Println(err)
		return
	}
	randomName := base64.RawURLEncoding.EncodeToString(randomArr)
	paramKey := aspect + randomName + ".mp4"
	putObjectParams.Key = &paramKey
	putObjectParams.Body = processedVideo
	putObjectParams.ContentType = &mimeType
	output, err := cfg.s3Client.PutObject(context.Background(), &putObjectParams)
	if err != nil {
		fmt.Println(err)
		respondWithError(w, http.StatusInternalServerError, "error with upload on cloud", err)
		return
	}
	fmt.Println(output)
	//videoURL := cfg.s3Bucket + "," + paramKey
	videoURL := "https://" + cfg.s3CfDistribution + "/" + paramKey
	videoData.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(videoData)
	if err != nil {
		fmt.Println(err)
		return
	}
}
