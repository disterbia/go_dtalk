package handler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
)

func VideoHandler(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		c.String(400, fmt.Sprintf("get form err: %s", err.Error()))
		return
	}

	// 임시 디렉토리 생성
	tmpDir, err := ioutil.TempDir("", "video")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 파일 저장
	src := fmt.Sprintf("%s/%s", tmpDir, file.Filename)
	if err := c.SaveUploadedFile(file, src); err != nil {
		c.String(500, fmt.Sprintf("upload file err: %s", err.Error()))
		return
	}

	// 파일 변환
	uniqueID := uuid.New()
	dst := fmt.Sprintf("%s/%s.m3u8", tmpDir, uniqueID)
	cmd := exec.Command("ffmpeg", "-i", src, "-profile:v", "baseline", "-level", "3.0", "-s", "640x360", "-start_number", "0", "-hls_time", "10", "-hls_list_size", "0", "-f", "hls", "-hls_segment_filename", fmt.Sprintf("%s/%%d-%s.ts", tmpDir, uniqueID), dst)
	err = cmd.Run()
	if err != nil {
		c.String(500, fmt.Sprintf("convert file err: %s", err.Error()))
		return
	}

	// 변환된 ts 파일들을 Firebase Storage에 업로드
	tsFiles, err := ioutil.ReadDir(tmpDir)
	if err != nil {
		c.String(500, fmt.Sprintf("read ts files err: %s", err.Error()))
		return
	}
	bucket := client.Bucket(bucketName)
	tsDownloadURLs := make(map[string]string)
	for _, f := range tsFiles {
		if strings.HasSuffix(f.Name(), ".ts") {
			tsFilePath := fmt.Sprintf("%s/%s", tmpDir, f.Name())
			objectPath := fmt.Sprintf("videos/%s-%s", uniqueID, f.Name())

			wc := bucket.Object(objectPath).NewWriter(context.Background())
			defer wc.Close()

			tsFile, err := os.Open(tsFilePath)
			if err != nil {
				c.String(500, fmt.Sprintf("open ts file err: %s", err.Error()))
				return
			}
			defer tsFile.Close()

			if _, err = io.Copy(wc, tsFile); err != nil {
				c.String(500, fmt.Sprintf("upload ts file err: %s", err.Error()))
				return
			}

			// ts 파일의 다운로드 URL 가져오기
			if err != nil {
				c.String(500, fmt.Sprintf("get file attrs err: %s", err.Error()))
				return
			}
			tsDownloadURL := fmt.Sprintf("https://storage.googleapis.com/%s%s%s", bucketName, "/", objectPath)
			tsDownloadURLs[f.Name()] = tsDownloadURL
		}
	}

	// .m3u8 파일 내의 상대 경로를 절대 경로 수정하기
	m3u8File, err := os.Open(dst)
	if err != nil {
		c.String(500, fmt.Sprintf("open m3u8 file err: %s", err.Error()))
		return
	}
	defer m3u8File.Close()

	m3u8ModifiedPath := fmt.Sprintf("%s/modified_output.m3u8", tmpDir)
	m3u8ModifiedFile, err := os.Create(m3u8ModifiedPath)
	if err != nil {
		c.String(500, fmt.Sprintf("create modified m3u8 file err: %s", err.Error()))
		return
	}
	defer m3u8ModifiedFile.Close()

	scanner := bufio.NewScanner(m3u8File)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasSuffix(line, ".ts") {
			line = tsDownloadURLs[line]
		}
		if _, err := m3u8ModifiedFile.WriteString(line + "\n"); err != nil {
			c.String(500, fmt.Sprintf("write to modified m3u8 file err: %s", err.Error()))
			return
		}
	}

	if err := scanner.Err(); err != nil {
		c.String(500, fmt.Sprintf("scan m3u8 file err: %s", err.Error()))
		return
	}

	// 수정된 .m3u8 파일을 Firebase Storage에 업로드
	objectPath := fmt.Sprintf("videos/%s-%s.m3u8", uniqueID, file.Filename)
	wc := bucket.Object(objectPath).NewWriter(context.Background())
	defer wc.Close()

	m3u8ModifiedFile.Seek(0, io.SeekStart)
	if _, err = io.Copy(wc, m3u8ModifiedFile); err != nil {
		c.String(500, fmt.Sprintf("upload modified m3u8 file err: %s", err.Error()))
		return
	}

	// 업로드된 파일의 다운로드 URL 가져오기
	downloadURL := fmt.Sprintf("https://storage.googleapis.com/%s%s%s", bucketName, "/", objectPath)
	if err != nil {
		c.String(500, fmt.Sprintf("get download URL err: %s", err.Error()))
		return
	}

	// 업로드 완료 후 다운로드 URL 반환
	c.JSON(200, gin.H{
		"message": "Upload success",
		"url":     downloadURL,
	})
}

type Video struct {
	Url string `json:"url"`
}

func ReadVideo(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "0")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid page number",
		})
		return
	}

	videos, err := getVideosFromStorage(page)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch videos",
		})
		return
	}

	if videos == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Page not found",
		})
		return
	}

	// Extract URLs from the Video structs
	videoUrls := make([]string, len(videos))
	for i, video := range videos {
		videoUrls[i] = video.Url
	}

	c.JSON(http.StatusOK, videoUrls)
}

func getVideosFromStorage(page int) ([]Video, error) {
	ctx := context.Background()

	bucket := client.Bucket(bucketName)

	query := &storage.Query{
		Prefix: "videos/",
	}
	objs := bucket.Objects(ctx, query)

	var videoUrls []string
	for {
		attrs, err := objs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		if strings.HasSuffix(attrs.Name, ".m3u8") {
			videoUrls = append(videoUrls, attrs.MediaLink)
		}
	}
	pageSize := 5
	startIndex := page * pageSize
	endIndex := startIndex + pageSize
	if endIndex > len(videoUrls) {
		endIndex = len(videoUrls)
	}

	if startIndex >= len(videoUrls) {
		return nil, nil
	}

	videos := make([]Video, endIndex-startIndex)
	for i := startIndex; i < endIndex; i++ {
		videos[i-startIndex] = Video{Url: videoUrls[i]}
	}

	return videos, nil
}
