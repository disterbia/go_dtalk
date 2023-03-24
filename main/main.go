package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"google.golang.org/api/option"
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

func init() {
	// Initialize Firebase app and storage client
	opt := option.WithCredentialsFile("./firebase_credentials.json")
	var err error
	app, err = firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Firebase app initialization error: %v\n", err)
		os.Exit(1)
	}
	client, err = app.Storage(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Firebase storage initialization error: %v\n", err)
		os.Exit(1)
	}

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
}

func main() {
	router := gin.Default()
	router.POST("/multiupload", handleImageMultiUpload)
	router.Run(":8080")
}

func handleImageMultiUpload(c *gin.Context) {
	var objects []ObjectData
	if err := c.BindJSON(&objects); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(objects) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No objects to upload"})
		return
	}

	bucketName := "oauthtest-8d82e.appspot.com"
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var imageURLs []string
	var wg sync.WaitGroup

	ch := make(chan string)

	for _, obj := range objects {
		for _, imageFile := range obj.ImageFiles {
			wg.Add(1)
			go func(imageFile string) {
				defer wg.Done()

				// Decode base64-encoded image data
				data, err := base64.StdEncoding.DecodeString(imageFile)
				if err != nil {
					ch <- err.Error()
					return
				}

				// Generate UUID for the file name
				uuid := uuid.New().String()
				ext := filepath.Ext(obj.Title)
				filename := uuid + ext

				// Upload file to Firebase storage
				object := bucket.Object(fmt.Sprintf("images/%s", filename))
				wc := object.NewWriter(context.Background())

				if _, err := wc.Write(data); err != nil {
					ch <- err.Error()
					return
				}

				if err := wc.Close(); err != nil {
					ch <- err.Error()
					return
				}

				imageURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, object.ObjectName())
				imageURLs = append(imageURLs, imageURL)
				ch <- imageURL
			}(imageFile)
		}
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var objectValues []string
	var imageURLValues []string
	var imageURLArgs []interface{}

	for _, obj := range objects {
		objectValues = append(objectValues, "(?, ?)")
		for _, _ = range obj.ImageFiles {
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
			imageURL := <-ch
			imageURLArgs = append(imageURLArgs, objectID, imageURL)
		}
		objectID++
	}

	stmt = fmt.Sprintf("INSERT INTO image_urls (object_id, url) VALUES %s", strings.Join(imageURLValues, ","))
	_, err = dbConnection.Exec(stmt, imageURLArgs...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"image_urls": imageURLs})
}

// import (
// 	"database/sql"
// 	"fmt"
// 	"mime/multipart"
// 	"net/http"
// 	"os"
// 	"path/filepath"
// 	"strings"
// 	"sync"

// 	"github.com/aws/aws-sdk-go/aws"
// 	"github.com/aws/aws-sdk-go/aws/session"
// 	"github.com/aws/aws-sdk-go/service/s3"
// 	"github.com/gin-gonic/gin"
// 	"github.com/go-sql-driver/mysql"
// 	"github.com/google/uuid"
// )

// var (
// 	s3Client     *s3.S3
// 	dbConnection *sql.DB
// 	bufferSize   = 1024 * 32 // 32 KB

// 	bufPool = sync.Pool{
// 		New: func() interface{} {
// 			return make([]byte, bufferSize)
// 		},
// 	}
// )

// func init() {
// 	// Initialize AWS S3 client
// 	sess, err := session.NewSession(&aws.Config{
// 		Region: aws.String("ap-northeast-2"), // Update with your S3 region
// 	})
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "AWS S3 client initialization error: %v\n", err)
// 		os.Exit(1)
// 	}
// 	s3Client = s3.New(sess)

// 	config := mysql.Config{
// 		User:                 "root",
// 		Passwd:               "freedom67",
// 		Net:                  "tcp",
// 		Addr:                 "localhost:3306",
// 		DBName:               "imgurl",
// 		AllowNativePasswords: true,
// 		ParseTime:            true,
// 	}
// 	dbConnection, err = sql.Open("mysql", config.FormatDSN())
// 	if err != nil {
// 		fmt.Fprintf(os.Stderr, "Database initialization error: %v\n", err)
// 		os.Exit(1)
// 	}
// }

// func main() {
// 	router := gin.Default()
// 	router.POST("/multiupload", handleImageMultiUpload)
// 	router.Run(":8080")
// }

// func handleImageMultiUpload(c *gin.Context) {
// 	form, _ := c.MultipartForm()
// 	files := form.File["images"]

// 	bucketName := "gobloctest"
// 	awsSession, err := session.NewSession(&aws.Config{
// 		Region: aws.String("ap-northeast-2"),
// 	})
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}
// 	s3Client := s3.New(awsSession)

// 	var imageURLs []string
// 	var wg sync.WaitGroup
// 	var ch = make(chan string, len(files))

// 	for _, file := range files {
// 		wg.Add(1)
// 		go func(file *multipart.FileHeader) {
// 			defer wg.Done()
// 			// Generate UUID for the file name
// 			uuid := uuid.New().String()
// 			ext := filepath.Ext(file.Filename)
// 			filename := uuid + ext

// 			// Upload file to S3
// 			f, err := file.Open()
// 			if err != nil {
// 				ch <- err.Error()
// 				return
// 			}
// 			defer f.Close()

// 			// Upload file to S3
// 			_, err = s3Client.PutObject(&s3.PutObjectInput{
// 				Bucket: aws.String(bucketName),
// 				Key:    aws.String("images/" + filename),
// 				Body:   f,
// 			})
// 			if err != nil {
// 				ch <- err.Error()
// 				return
// 			}

// 			// Get public URL of uploaded file
// 			imageURL := fmt.Sprintf("https://%s.s3-%s.amazonaws.com/images/%s", bucketName, *awsSession.Config.Region, filename)
// 			ch <- imageURL
// 		}(file)
// 	}
// 	go func() {
// 		wg.Wait()
// 		close(ch)
// 	}()

// 	for imageURL := range ch {
// 		if imageURL == "" {
// 			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload image"})
// 			return
// 		}
// 		imageURLs = append(imageURLs, imageURL)
// 	}

// 	var valueStrings []string
// 	var valueArgs []interface{}
// 	for _, url := range imageURLs {
// 		valueStrings = append(valueStrings, "(?)")
// 		valueArgs = append(valueArgs, url)
// 	}
// 	stmt := fmt.Sprintf("INSERT INTO temp (imgurl) VALUES %s", strings.Join(valueStrings, ","))
// 	_, err = dbConnection.Exec(stmt, valueArgs...)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}

// 	c.JSON(http.StatusOK, gin.H{"image_urls": imageURLs})
// }
