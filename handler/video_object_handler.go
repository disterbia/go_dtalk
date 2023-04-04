package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
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
	LikesCount int    `json:"likesCount"`
}

func VideoObjectHandler(c *gin.Context) {

	// 메타데이터를 파싱합니다.
	metadata := c.PostForm("metadata")
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

	var wg sync.WaitGroup
	wg.Add(len(files))

	results := make(chan gin.H, len(files))

	for _, file := range files {
		//videoObject := videoObjects[index]

		go func(file *multipart.FileHeader) {
			defer wg.Done()
			// 파일 저장
			src := fmt.Sprintf("%s/%s", tmpDir, file.Filename)
			if err := c.SaveUploadedFile(file, src); err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("upload file err: %s", err.Error())}
				return
			}

			// 파일 변환
			uniqueID := uuid.New()
			dst := fmt.Sprintf("%s/%s.m3u8", tmpDir, uniqueID)
			cmd := exec.Command("ffmpeg", "-i", src, "-profile:v", "baseline", "-level", "3.0", "-s", "640x360", "-start_number", "0", "-hls_time", "10", "-hls_list_size", "0", "-f", "hls", "-hls_segment_filename", fmt.Sprintf("%s/%%d-%s.ts", tmpDir, uniqueID), dst)
			err = cmd.Run()
			if err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("convert file err: %s", err.Error())}
				return
			}

			// 변환된 ts 파일들을 Firebase Storage에 업로드
			tsFiles, err := ioutil.ReadDir(tmpDir)
			if err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("read ts files err: %s", err.Error())}
				return
			}

			tsDownloadURLs := make(map[string]string)

			var tsUploadWg sync.WaitGroup
			tsUploadWg.Add(len(tsFiles))

			for _, f := range tsFiles {
				go func(f os.FileInfo) {
					defer tsUploadWg.Done()

					if strings.HasSuffix(f.Name(), ".ts") {
						tsFilePath := fmt.Sprintf("%s/%s", tmpDir, f.Name())
						objectPath := fmt.Sprintf("videos/%s-%s", uniqueID, f.Name())

						tsFile, err := os.Open(tsFilePath)
						if err != nil {
							results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("open ts file err: %s", err.Error())}
							return
						}
						defer tsFile.Close()

						pr, pw := io.Pipe()
						go func() {
							defer pw.Close()
							io.Copy(pw, tsFile)
						}()

						wc := bucket.Object(objectPath).NewWriter(context.Background())
						defer wc.Close()

						if _, err = io.Copy(wc, pr); err != nil {
							results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("upload ts file err: %s", err.Error())}
							return
						}

						// ts 파일의 다운로드URL 가져오기
						tsDownloadURL := fmt.Sprintf("https://storage.googleapis.com/%s%s%s", bucketName, "/", objectPath)
						tsDownloadURLs[f.Name()] = tsDownloadURL
					}
				}(f)
			}

			tsUploadWg.Wait()

			// .m3u8 파일 내의 상대 경로를 절대 경로 수정하기
			m3u8File, err := os.Open(dst)
			if err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("open m3u8 file err: %s", err.Error())}
				return
			}
			defer m3u8File.Close()

			m3u8ModifiedPath := fmt.Sprintf("%s/modified_output.m3u8", tmpDir)
			m3u8ModifiedFile, err := os.Create(m3u8ModifiedPath)
			if err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("create modified m3u8 file err: %s", err.Error())}
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
					results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("write to modified m3u8 file err: %s", err.Error())}
					return
				}
			}

			if err := scanner.Err(); err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("scan m3u8 file err: %s", err.Error())}
				return
			}

			// 수정된 .m3u8 파일을 Firebase Storage에 업로드
			objectPath := fmt.Sprintf("videos/%s-%s.m3u8", uniqueID, file.Filename)
			wc := bucket.Object(objectPath).NewWriter(context.Background())
			defer wc.Close()

			m3u8ModifiedFile.Seek(0, io.SeekStart)
			if _, err = io.Copy(wc, m3u8ModifiedFile); err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("upload modified m3u8 file err: %s", err.Error())}
				return
			}

			// 업로드된 파일의 다운로드 URL 가져오기
			downloadURL := fmt.Sprintf("https://storage.googleapis.com/%s%s%s", bucketName, "/", objectPath)
			if err != nil {
				results <- gin.H{"file": file.Filename, "error": fmt.Sprintf("get download URL err: %s", err.Error())}
				return
			}
			// 업로드된 동영상 정보 firebase database에 저장
			if err != nil {
				log.Fatalf("Failed to create firestore client: %v", err)
			}
			defer dbClient.Close()

			_, _, err = dbClient.Collection("videos").Add(context.Background(), map[string]interface{}{
				"title":       videoObjects[0].Title,
				"uploader":    videoObjects[0].Uploader,
				"url":         downloadURL,
				"upload_time": time.Now(),
				"likes":       0,
			})

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
}
