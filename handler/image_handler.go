package handler

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"image"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/nfnt/resize"
	"google.golang.org/api/option"

	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go"
)

const (
	maxWorkers  = 10
	bucketName  = "oauthtest-8d82e.appspot.com"
	imageWidth  = 1024
	imageBucket = "images/"
	privateKey  = "-----BEGIN PRIVATE KEY-----\nMIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQCQA1qrXp5p9vHp\ntgjaWFMtuYzBkjiGfzn4qvBlwcWMbC3fp8TS9Dhtjw12s+XUFTz+zjR+IRzTfg/k\ncEFI70uWdqgVr1EhzXLR0a53PLpSnu8FQ3VcrC6HBGLg01bsbyGVnLCAM3D3mmpN\nkeGZ0opOkWoEarUXqGSStSxt5hSKlZK8I2j+fFKb7JMzuW7eAFsLapVRHdidMrv4\nMlEHR+cvA1q5CRj3QkvSKdIYUeaq9wDA24ZIR0ppdiCJ2ez6n/uLpRxtT59XjSFw\nvWaumgTCFEIa9VRVelLKP9y6TM0xBSb9Aohb0MogdX6/KGAmX5E4HLw5NKLJHOGM\nyIgH+VeZAgMBAAECggEAAy+lhoc3urgAtbYM76lv4IaZLIrisR5/m1a40rHKtwJA\nRCOKbLpfe8tRyCZ0CZo51E9VcrAQbSEipQSue+9jYacrG0FtDwRR9Mdp4hRzXo4m\nCgpqPl6P+T6XYkpoZ7G03ya2hKnesE8cVkCSxfeD2EEBMoaIadmB9mb1TAlucekB\nK6LhamIheifSkVuTnBStlEPX+0H3aP4W817c0KGG/EdVkoLjfIBeWSP5xMOBSJJT\nN2PJkaNsk7O7ywFkJxrgLbKiGlAfm5jBbbmW9kAQTa9i8/4PbhIF6sPTcelAiEyL\nNJ7mwaVqdUH/A68P8NLpxTptIvTmW4I1u6Ar0iHByQKBgQDEOh4778MnG1milI81\nenbB1nYx+u8MsXxvBlbikAboE3nMVaS+euvNwIO3mAkpT2lW9vQ5UOTpsgmJd/l/\nlQQBzzTQxwgmv5Qtp+BuOD4sCCb2pU0BdXbSGlVPzLyO1648ForzaygyOq/1ujPe\nLbiJND3Njh7xRp9ue/IJwqPI/QKBgQC74ZF4JmZC+NjUvmp1yFPyQPlwheA8Qhz+\nOyZsoC5Cs4/9OESJAOjwI1LIdYqTZf02ENrcRVZCDGIZJxTzi7DcSbhhRmrY+2Mz\nCNZ+PDCxfGZg0c6c+206DMtmpe7z9SQBGu/neUK8x9cRSs8eMmncmBitxx0UJcI6\nQUUrEeqJzQKBgGKOHilUXtwBbJ+vpc3iWEs6/9pSgkYJzsmkkXbxh8aAIahzS28w\nJccNbhqEDfXloK7BEiDHdHG7rfaRf4qIuZ5/B7Pkgz+S8UWND7fMH83Vulwe4fJd\noPQdrcOKvRmxUh1z5Q4lP+caes4cW3i31ftzdacMPpZINkMzlXk5fTGxAoGAKY0j\nfO0RJLKgUbyjEtVxK1yPTgFtrCX6/4bZYqCyWnIX4Cq3jY0z9xf40Pid4ydlLrXf\nkWOMRiMy9tkb2xkDzlRHgMvwCXjfYYQM2/I32qjmg3cjOLiqWXJG8ba0+CM5CT2J\n3SmGRvXzbJGc6NLBctX4b0Zf+fq3z+Zrg7D8q+kCgYBZ7Sx0BzUlR12M/CRLUlvb\nzv8dkNi8N6CBWMbeGkW5IaEEvuurRfESibe/ok2y0rXbRm0iKuEiQPmIqkdhSgE/\nqgs1uAAst/1PukbCVXc7qX3DaiCI/x1xCgsZ7BL3+nlRkcfprkk4oM621O3B1etm\nxMgmrwd56OK6LqeGN5dcvg==\n-----END PRIVATE KEY-----\n"
)

var (
	app          *firebase.App
	client       *storage.Client
	dbConnection *sql.DB
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

func init() {
	// Initialize Firebase app and storage client
	opt := option.WithCredentialsFile("./firebase_credentials.json")
	var err error
	app, err = firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Firebase app initialization error: %v\n", err)
		os.Exit(1)
	}
	storageClient, err := storage.NewClient(context.Background(), opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Firebase storage initialization error: %v\n", err)
		os.Exit(1)
	}
	client = storageClient
	config := mysql.Config{
		User:                 "root",
		Passwd:               "freedom67",
		Net:                  "tcp",
		Addr:                 "localhost:3306",
		DBName:               "imgurl",
		AllowNativePasswords: true,
		ParseTime:            true,
	}
	dbConnection, err = sql.Open("mysql", config.FormatDSN())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Database initialization error: %v\n", err)
		os.Exit(1)
	}

	// Set up the database connection pool
	dbConnection.SetMaxIdleConns(10)
	dbConnection.SetMaxOpenConns(20)
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
