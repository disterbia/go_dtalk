package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type VideoObject struct {
	Title    string `json:"title"`
	Uploader string `json:"uploader"`
}

type VideoData struct {
	Title      string `json:"title"`
	Uploader   string `json:"uploader"`
	URL        string `json:"url"`
	UploadTime string `json:"uploadTime"`
	LikeCount  int    `json:"likeCount"`
}

func VideoObjectHandler(c *gin.Context) {
	// 메타데이터를 파싱합니다.
	println("start")
	start := time.Now()

	metadata := c.PostForm("metadata")

	elapsed := time.Since(start)
	fmt.Printf("Time to get raw data: %s\n", elapsed)

	fmt.Println(metadata)
	var videoObjects []VideoObject
	err := json.Unmarshal([]byte(metadata), &videoObjects)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 파일을 가져옵니다.
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	files := form.File["files"]

	// 임시 디렉토리 생성
	tmpDir, err := ioutil.TempDir("", "video")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	results := make(chan gin.H, len(files))
	var wg sync.WaitGroup
	wg.Add(len(files))

	for _, file := range files {
		go func(file *multipart.FileHeader) {
			defer wg.Done()

			uniqueID := uuid.New()

			src := filepath.Join(tmpDir, file.Filename)
			dst := filepath.Join(tmpDir, fmt.Sprintf("%s.m3u8", uniqueID))

			if err := c.SaveUploadedFile(file, src); err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("upload file err: %s", err.Error())}
				return
			}

			if err := convertVideo(src, dst, uniqueID, tmpDir); err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("convert file err: %s", err.Error())}
				return
			}

			tsDownloadURLs, err := uploadTsFiles(tmpDir, uniqueID)
			if err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("upload ts files err: %s", err.Error())}
				return
			}

			if err := modifyM3U8File(dst, tmpDir, tsDownloadURLs); err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("modify m3u8 file err: %s", err.Error())}
				return
			}

			downloadURL, err := uploadM3U8File(tmpDir, file, uniqueID)
			if err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("upload m3u8 file err: %s", err.Error())}
				return
			}

			thumbnailURL, err := uploadThumbnail(tmpDir, uniqueID)
			if err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("upload thumbnail err: %s", err.Error())}
				return
			}
			err = uploadVideoInfoToFirestore(videoObjects[0], downloadURL, thumbnailURL)
			if err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("upload video info to firestore err: %s", err.Error())}
				return
			}

			// 업로드 완료 후 다운로드 URL 반환
			results <- gin.H{"file": file.Filename, "url": downloadURL}
		}(file)
	}

	wg.Wait()
	close(results)

	responseData := make([]gin.H, 0, len(files))
	for result := range results {
		responseData = append(responseData, result)
	}

	c.JSON(200, responseData)
	elapsed2 := time.Since(start)
	fmt.Printf("Time to end: %s\n", elapsed2)
}

func convertVideo(src, dst string, uniqueID uuid.UUID, tmpDir string) error {
	thumbnailPath := filepath.Join(tmpDir, fmt.Sprintf("%s-thumbnail.webp", uniqueID))
	thumbnailCmd := exec.Command("ffmpeg", "-i", src, "-vf", "fps=10,scale=480:640:flags=lanczos", "-ss", "0", "-t", "2", "-loop", "0", "-c:v", "libwebp", "-preset", "default", "-an", "-vsync", "0", "-q:v", "60", thumbnailPath)
	if err := thumbnailCmd.Run(); err != nil {
		return err
	}
	cmd := exec.Command("ffmpeg", "-i", src, "-profile:v", "baseline", "-level", "3.0", "-s", "640x360", "-start_number", "0", "-hls_time", "10", "-hls_list_size", "0", "-f", "hls", "-hls_segment_filename", fmt.Sprintf("%s/%%d-%s.ts", filepath.Dir(dst), uniqueID), dst)

	return cmd.Run()
}

func uploadTsFiles(tmpDir string, uniqueID uuid.UUID) (map[string]string, error) {
	tsFiles, err := ioutil.ReadDir(tmpDir)
	if err != nil {
		return nil, err
	}

	tsDownloadURLs := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(tsFiles))

	for _, f := range tsFiles {
		go func(f os.FileInfo) {
			defer wg.Done()

			if strings.HasSuffix(f.Name(), ".ts") {
				tsFilePath := filepath.Join(tmpDir, f.Name())
				objectPath := fmt.Sprintf("videos/%s-%s", uniqueID, f.Name())

				tsFile, err := os.Open(tsFilePath)
				if err != nil {
					return
				}
				defer tsFile.Close()

				pr, pw := io.Pipe()
				go func() {
					defer pw.Close()
					io.Copy(pw, tsFile)
				}()

				wc := bucket.Object(objectPath).NewWriter(ctx)
				defer wc.Close()

				if _, err = io.Copy(wc, pr); err != nil {
					return
				}

				// ts 파일의 다운로드URL 가져오기
				tsDownloadURL := fmt.Sprintf("https://storage.googleapis.com/%s%s%s", bucketName, "/", objectPath)
				mu.Lock()
				tsDownloadURLs[f.Name()] = tsDownloadURL
				mu.Unlock()
			}
		}(f)
	}

	wg.Wait()

	return tsDownloadURLs, nil
}

func modifyM3U8File(dst, tmpDir string, tsDownloadURLs map[string]string) error {
	m3u8File, err := os.Open(dst)
	if err != nil {
		return err
	}
	defer m3u8File.Close()

	m3u8ModifiedPath := filepath.Join(tmpDir, "modified_output.m3u8")
	m3u8ModifiedFile, err := os.Create(m3u8ModifiedPath)
	if err != nil {
		return err
	}
	defer m3u8ModifiedFile.Close()

	scanner := bufio.NewScanner(m3u8File)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasSuffix(line, ".ts") {
			line = tsDownloadURLs[line]
		}
		if _, err := m3u8ModifiedFile.WriteString(line + "\n"); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

func uploadM3U8File(tmpDir string, file *multipart.FileHeader, uniqueID uuid.UUID) (string, error) {
	m3u8ModifiedPath := filepath.Join(tmpDir, "modified_output.m3u8")
	m3u8ModifiedFile, err := os.Open(m3u8ModifiedPath)
	if err != nil {
		return "", err
	}
	defer m3u8ModifiedFile.Close()

	objectPath := fmt.Sprintf("videos/%s-%s.m3u8", uniqueID, file.Filename)
	wc := bucket.Object(objectPath).NewWriter(ctx)
	defer wc.Close()

	if _, err = io.Copy(wc, m3u8ModifiedFile); err != nil {
		return "", err
	}

	// 업로드된 파일의 다운로드 URL 가져오기
	downloadURL := fmt.Sprintf("https://storage.googleapis.com/%s%s%s", bucketName, "/", objectPath)
	return downloadURL, nil
}

func uploadThumbnail(tmpDir string, uniqueID uuid.UUID) (string, error) {
	thumbnailPath := filepath.Join(tmpDir, fmt.Sprintf("%s-thumbnail.webp", uniqueID))
	thumbnailFile, err := os.Open(thumbnailPath)
	if err != nil {
		return "", err
	}
	defer thumbnailFile.Close()

	objectPath := fmt.Sprintf("thumbnails/%s-thumbnail.webp", uniqueID)
	wc := bucket.Object(objectPath).NewWriter(ctx)
	defer wc.Close()

	if _, err = io.Copy(wc, thumbnailFile); err != nil {
		return "", err
	}

	thumbnailURL := fmt.Sprintf("https://storage.googleapis.com/%s%s%s", bucketName, "/", objectPath)
	return thumbnailURL, nil
}
func uploadVideoInfoToFirestore(videoObject VideoObject, downloadURL, thumbnailURL string) error {

	_, _, err := dbClient.Collection("videos").Add(ctx, map[string]interface{}{
		"title":       videoObject.Title,
		"uploader":    videoObject.Uploader,
		"url":         downloadURL,
		"thumbnail":   thumbnailURL, // Add thumbnail URL
		"upload_time": time.Now(),
		"like_count":  0,
	})

	return err
}
