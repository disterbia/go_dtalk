package handler

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func VideoHandler(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.String(400, fmt.Sprintf("get form err: %s", err.Error()))
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

type Video struct {
	Title       string   `firestore:"title" json:"title"`
	Uploader    string   `firestore:"uploader" json:"uploader"`
	Url         string   `firestore:"url" json:"url"`
	LikeCount   int      `firestore:"like_count" json:"like_count"`
	Upload_time string   `firestore:"upload_time" json:"upload_time"`
	Thumbnail   string   `firestore:"thumbnail" json:"thumbnail"`
	UserInfo    UserInfo `json:"user_info"`
}

type UserInfo struct {
	Id    string `firestore:"id" json:"id"`
	Image string `firestore:"image" json:"image"`
}

func ReadUserVideos(c *gin.Context) {
	userID := c.DefaultQuery("user_id", "")

	videos, err := getUserVideosFromDatabase(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to fetch user videos: %v", err),
		})
		return
	}

	if videos == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No videos found",
		})
		return
	}

	c.JSON(http.StatusOK, videos)
}

func getUserVideosFromDatabase(userID string) ([]Video, error) {
	var videos []Video
	docs, err := dbClient.Collection("videos").Where("uploader", "==", userID).OrderBy("upload_time", firestore.Desc).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		video := Video{
			Title:       doc.Data()["title"].(string),
			Uploader:    doc.Data()["uploader"].(string),
			Url:         doc.Data()["url"].(string),
			LikeCount:   int(doc.Data()["like_count"].(int64)),
			Upload_time: doc.Data()["upload_time"].(time.Time).Format(time.RFC3339),
			Thumbnail:   doc.Data()["thumbnail"].(string),
		}
		videos = append(videos, video)
	}

	return videos, nil
}

func ReadVideo(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "0")
	videoStr := c.DefaultQuery("first", "")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid page number",
		})
		return
	}

	pageSize := 1
	if page == 0 {
		pageSize = 3
	}

	videos, err := getVideosFromDatabase(page, pageSize, videoStr)
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

	c.JSON(http.StatusOK, videos)
}

func getVideosFromDatabase(page int, pageSize int, videoStr string) ([]Video, error) {
	ctx := context.Background()
	var videos []Video

	// Get the total count of videos
	docs, err := dbClient.Collection("videos").OrderBy("upload_time", firestore.Desc).Offset(page).Limit(pageSize).Documents(ctx).GetAll()
	firstdoc, err2 := dbClient.Collection("videos").OrderBy("upload_time", firestore.Desc).Offset(0).Limit(1).Documents(ctx).GetAll()
	// userVideo, err := dbClient.Collection("videos").Where("uploader", "==", userID).OrderBy("upload_time", firestore.Desc).Documents(ctx).GetAll()

	// docs, err := dbClient.Collection("videos").Offset(2).Limit(4).Documents(ctx).GetAll()
	if err != nil {
		print(1111)
		return nil, err
	}
	if err2 != nil {
		print(2222)
		return nil, err2
	}
	totalCount := len(docs)

	// Calculate the number of random documents needed
	if pageSize > totalCount {
		print(3333)
		return nil, nil
	}

	if firstdoc[0].Data()["url"].(string) != videoStr && page != 0 {

		userId := firstdoc[0].Data()["uploader"].(string)
		user, err := dbClient.Collection("users").Doc(userId).Get(ctx)
		if err != nil {
			return nil, err
		}
		userInfo := UserInfo{
			Id:    user.Data()["id"].(string),
			Image: user.Data()["image"].(string),
		}
		video := Video{
			Title:       firstdoc[0].Data()["title"].(string),
			Uploader:    firstdoc[0].Data()["uploader"].(string),
			Url:         firstdoc[0].Data()["url"].(string),
			Upload_time: firstdoc[0].Data()["upload_time"].(time.Time).Format(time.RFC3339),
			LikeCount:   int(firstdoc[0].Data()["like_count"].(int64)),
			UserInfo:    userInfo,
		}
		videos = append(videos, video)

	} else {
		for i := 0; i < pageSize; i++ {
			doc := docs[i]
			userId := doc.Data()["uploader"].(string)
			user, err := dbClient.Collection("users").Doc(userId).Get(ctx)
			if err != nil {
				return nil, err
			}
			userInfo := UserInfo{
				Id:    user.Data()["id"].(string),
				Image: user.Data()["image"].(string),
			}
			video := Video{
				Title:       doc.Data()["title"].(string),
				Uploader:    doc.Data()["uploader"].(string),
				Url:         doc.Data()["url"].(string),
				LikeCount:   int(doc.Data()["like_count"].(int64)),
				Upload_time: doc.Data()["upload_time"].(time.Time).Format(time.RFC3339),
				UserInfo:    userInfo,
			}
			videos = append(videos, video)
		}
	}

	return videos, nil
}

// type Video struct {
// 	Url string `json:"url"`
// }

// func ReadInitialVideo(c *gin.Context) {

// 	videos, err := getInitialVideosFromStorage(0)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"error": "Failed to fetch videos",
// 		})
// 		return
// 	}

// 	if videos == nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"error": "Page not found",
// 		})
// 		return
// 	}

// 	// Extract URLs from the Video structs
// 	videoUrls := make([]string, len(videos))
// 	for i, video := range videos {
// 		videoUrls[i] = video.Url
// 	}

// 	c.JSON(http.StatusOK, videoUrls)
// }

// func getInitialVideosFromStorage(page int) ([]Video, error) {
// 	ctx := context.Background()

// 	bucket := client.Bucket(bucketName)

// 	query := &storage.Query{
// 		Prefix: "videos/",
// 	}
// 	objs := bucket.Objects(ctx, query)

// 	var videoUrls []string
// 	for {
// 		attrs, err := objs.Next()
// 		if err == iterator.Done {
// 			break
// 		}
// 		if err != nil {
// 			return nil, err
// 		}

// 		if strings.HasSuffix(attrs.Name, ".m3u8") {
// 			videoUrls = append(videoUrls, attrs.MediaLink)
// 		}
// 	}
// 	fmt.Println(len(videoUrls))
// 	pageSize := 3
// 	startIndex := page * pageSize
// 	endIndex := startIndex + pageSize
// 	if endIndex > len(videoUrls) {
// 		endIndex = len(videoUrls)
// 	}

// 	if startIndex >= len(videoUrls) {
// 		return nil, nil
// 	}

// 	videos := make([]Video, endIndex-startIndex)
// 	for i := startIndex; i < endIndex; i++ {
// 		videos[i-startIndex] = Video{Url: videoUrls[i]}
// 	}

// 	return videos, nil
// }

// func ReadVideo(c *gin.Context) {
// 	pageStr := c.DefaultQuery("page", "0")
// 	page, err := strconv.Atoi(pageStr)
// 	if err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"error": "Invalid page number",
// 		})
// 		return
// 	}

// 	videos, err := getVideosFromStorage(page)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{
// 			"error": "Failed to fetch videos",
// 		})
// 		return
// 	}

// 	if videos == nil {
// 		c.JSON(http.StatusBadRequest, gin.H{
// 			"error": "Page not found",
// 		})
// 		return
// 	}

// 	// Extract URLs from the Video structs
// 	videoUrls := make([]string, len(videos))
// 	for i, video := range videos {
// 		videoUrls[i] = video.Url
// 	}

// 	c.JSON(http.StatusOK, videoUrls)
// }

// func getVideosFromStorage(page int) ([]Video, error) {
// 	ctx := context.Background()

// 	bucket := client.Bucket(bucketName)

// 	query := &storage.Query{
// 		Prefix: "videos/",
// 	}
// 	objs := bucket.Objects(ctx, query)

// 	var videoUrls []string
// 	for {
// 		attrs, err := objs.Next()
// 		if err == iterator.Done {
// 			break
// 		}
// 		if err != nil {
// 			return nil, err
// 		}

// 		if strings.HasSuffix(attrs.Name, ".m3u8") {
// 			videoUrls = append(videoUrls, attrs.MediaLink)
// 		}
// 	}
// 	fmt.Println(len(videoUrls))
// 	pageSize := 3
// 	startIndex := page * pageSize
// 	endIndex := startIndex + pageSize
// 	if endIndex > len(videoUrls) {
// 		endIndex = len(videoUrls)
// 	}

// 	if startIndex >= len(videoUrls) {
// 		return nil, nil
// 	}

// 	videos := make([]Video, endIndex-startIndex)
// 	for i := startIndex; i < endIndex; i++ {
// 		videos[i-startIndex] = Video{Url: videoUrls[i]}
// 	}

// 	return videos, nil
// }
