package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"net/http"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nfnt/resize"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
)

type Image struct {
	UserId     string   `json:"user_id"`
	ImageFiles []string `json:"imageFiles"`
}

type WorkRequest struct {
	imageFile string
	resultsCh chan<- string
	errorsCh  chan<- error
}

func worker(workChan <-chan WorkRequest, wg *sync.WaitGroup) {
	defer wg.Done()

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

	imageURL := fmt.Sprintf("https://storage.googleapis.com/%s%s%s", bucketName, "/images/", filename)
	return imageURL, nil
}

func HandleImageMultiUpload(c *gin.Context) {
	var objects []Image
	println("start")
	start := time.Now()

	if err := c.BindJSON(&objects); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	elapsed := time.Since(start)
	fmt.Printf("Time to get raw data: %s\n", elapsed)
	if len(objects) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No objects to upload"})
		return
	}

	resultsCh := make(chan string, len(objects)*maxWorkers)
	errorsCh := make(chan error, len(objects)*maxWorkers)
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

	for _, obj := range objects {
		// Update user's profileImage in Firestore
		userDocRef := dbClient.Collection("users").Doc(obj.UserId)
		_, err := userDocRef.Update(ctx, []firestore.Update{
			{Path: "image", Value: imageURLs[0]},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Add image data to Firestore
		imageRef := dbClient.Collection("images").NewDoc()
		_, err = imageRef.Set(ctx, map[string]interface{}{
			"userId":     obj.UserId,
			"url":        imageURLs[0],
			"uploadDate": time.Now(),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		//imageURLs = imageURLs[1:]
	}
	result := imageURLs[0]
	c.JSON(http.StatusOK, gin.H{"image_url": result})
	elapsed2 := time.Since(start)
	fmt.Printf("Time to end : %s\n", elapsed2)
}
