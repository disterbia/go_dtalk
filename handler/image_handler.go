package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"net/http"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nfnt/resize"

	"cloud.google.com/go/storage"
)

type ObjectData struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ImageFiles  []string `json:"imageFiles"`
}

type WorkRequest struct {
	imageFile string
	resultsCh chan<- string
	errorsCh  chan<- error
}

func worker(workChan <-chan WorkRequest, wg *sync.WaitGroup) {
	defer wg.Done()

	bucket := client.Bucket(bucketName)

	for work := range workChan {
		imageURL, err := processImage(bucket, work.imageFile)
		if err != nil {
			work.errorsCh <- err
			continue
		}
		work.resultsCh <- imageURL
	}
}

func processImage(bucket *storage.BucketHandle, imageFile string) (string, error) {
	// Decode base64-encoded image data
	data, err := base64.StdEncoding.DecodeString(imageFile)
	if err != nil {
		return "", fmt.Errorf("failed to decode image file: %w", err)
	}

	// Decode image
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize image
	resizedImg := resize.Resize(uint(imageWidth), 0, img, resize.Lanczos3)

	// Optimize image
	buf := new(bytes.Buffer)
	err = imaging.Encode(buf, resizedImg, imaging.JPEG, imaging.JPEGQuality(80))
	if err != nil {
		return "", fmt.Errorf("failed to optimize image: %w", err)
	}
	optimizeImg := buf.Bytes()

	// Generate UUID for the file name
	filename := uuid.New().String() + ".jpg"

	// Upload file to Firebase storage
	object := bucket.Object(imageBucket + filename)
	wc := object.NewWriter(context.Background())

	// Use a buffered writer for improved performance
	bufW := bufio.NewWriterSize(wc, 64*1024)

	if _, err := bufW.Write(optimizeImg); err != nil {
		return "", fmt.Errorf("failed to write image: %w", err)
	}

	if err := bufW.Flush(); err != nil {
		return "", fmt.Errorf("failed to flush buffer: %w", err)
	}

	if err := wc.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	imageURL := fmt.Sprintf("https://storage.googleapis.com/%s%s%s", bucketName, "/", filename)
	return imageURL, nil
}

func HandleImageMultiUpload(c *gin.Context) {
	var objects []ObjectData
	if err := c.BindJSON(&objects); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(objects) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No objects to upload"})
		return
	}

	resultsCh := make(chan string, len(objects)*maxWorkers)
	errorsCh := make(chan error, len(objects)*maxWorkers) // 버퍼 크기를 늘려 에러를 처리합니다.
	workChan := make(chan WorkRequest, maxWorkers)
	var wg sync.WaitGroup

	// Start worker pool
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			worker(workChan, &wg)
		}()
	}

	// Dispatch work
	go func() {
		for _, obj := range objects {
			for _, imageFile := range obj.ImageFiles {
				workChan <- WorkRequest{imageFile: imageFile, resultsCh: resultsCh, errorsCh: errorsCh}
			}
		}
		close(workChan)
	}()

	// Collect results and errors
	var imageURLs []string
	var anyErrors []error
	completed := 0
	for completed < len(objects)*len(objects[0].ImageFiles) {
		select {
		case imageURL := <-resultsCh:
			imageURLs = append(imageURLs, imageURL)
			completed++
		case err := <-errorsCh:
			anyErrors = append(anyErrors, err)
			completed++
		}
	}

	if len(anyErrors) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"errors": anyErrors})
		return
	}

	var objectValues []string
	var imageURLValues []string
	var imageURLArgs []interface{}

	for _, obj := range objects {
		objectValues = append(objectValues, "(?, ?)")
		for range obj.ImageFiles {
			imageURLValues = append(imageURLValues, "(?, ?)")
		}
	}

	stmt := fmt.Sprintf("INSERT INTO object_data (title, description) VALUES %s", strings.Join(objectValues, ","))
	args := []interface{}{}

	for _, obj := range objects {
		args = append(args, obj.Title, obj.Description)
	}

	result, err := dbConnection.Exec(stmt, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	objectID, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for _, obj := range objects {
		for range obj.ImageFiles {
			if len(imageURLs) > 0 {
				imageURL := imageURLs[0]
				imageURLs = imageURLs[1:]
				imageURLArgs = append(imageURLArgs, objectID, imageURL)
			} else {
				break
			}
		}
		objectID++ // objectID 값을 올바르게 증가시킵니다.
	}

	stmt = fmt.Sprintf("INSERT INTO image_urls (object_id, url) VALUES %s", strings.Join(imageURLValues, ","))
	_, err = dbConnection.Exec(stmt, imageURLArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"image_urls": imageURLArgs})
}
