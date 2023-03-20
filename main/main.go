package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"

	firebase "firebase.google.com/go"
	"firebase.google.com/go/storage"
	"github.com/gin-gonic/gin"
	"github.com/go-sql-driver/mysql"
	"google.golang.org/api/option"
)

var (
	app          *firebase.App
	client       *storage.Client
	dbConnection *sql.DB
)

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
	form, _ := c.MultipartForm()
	files := form.File["images"]
	bucketName := "oauthtest-8d82e.appspot.com"
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var imageURLs []string
	for _, file := range files {
		// Upload file to Firebase storage

		object := bucket.Object(fmt.Sprintf("images/%s", file.Filename))
		wc := object.NewWriter(context.Background())
		src, err := file.Open()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		defer src.Close()

		buf := bufio.NewReader(src)
		if _, err := io.CopyBuffer(wc, buf, make([]byte, 1024*32)); err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		if err := wc.Close(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Get public URL of uploaded file
		attrs, err := object.Attrs(context.Background())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		imageURLs = append(imageURLs, attrs.MediaLink)
	}

	stmt, err := dbConnection.Prepare("INSERT INTO temp (imgurl) VALUES (?)")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer stmt.Close()

	for _, url := range imageURLs {
		_, err = stmt.Exec(url)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"image_urls": imageURLs})
}

// func handleImageUpload(c *gin.Context) {
// 	file, err := c.FormFile("image")
// 	if err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file"})
// 		return
// 	}

// 	// Upload file to Firebase storage
// 	bucketName := "oauthtest-8d82e.appspot.com"
// 	bucket, err := client.Bucket(bucketName)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error6": err.Error()})
// 		return
// 	}

// 	object := bucket.Object(fmt.Sprintf("images/%s", file.Filename))
// 	wc := object.NewWriter(context.Background())
// 	//defer wc.Close()
// 	src, err := file.Open()
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error7": err.Error()})
// 		return
// 	}
// 	defer src.Close()

// 	buf := bufio.NewReader(src)
// 	if _, err := io.CopyBuffer(wc, buf, make([]byte, 1024*32)); err != nil {
// 		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "File write error"})
// 		return
// 	}

// 	// Get public URL of uploaded file
// 	if err := wc.Close(); err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error9": err.Error()})
// 		return
// 	}
// 	attrs, err := object.Attrs(context.Background())
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error10": err.Error()})
// 		return
// 	}
// 	c.JSON(http.StatusOK, gin.H{"image_url": attrs.MediaLink})
// }
