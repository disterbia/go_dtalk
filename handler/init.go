package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	firebase "firebase.google.com/go"
	"github.com/go-sql-driver/mysql"
	"google.golang.org/api/option"
)

var (
	maxWorkers  = 10
	bucketName  = "oauthtest-8d82e.appspot.com"
	imageWidth  = 1024
	imageBucket = "images/"
	privateKey  = "-----BEGIN PRIVATE KEY-----\nMIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQCQA1qrXp5p9vHp\ntgjaWFMtuYzBkjiGfzn4qvBlwcWMbC3fp8TS9Dhtjw12s+XUFTz+zjR+IRzTfg/k\ncEFI70uWdqgVr1EhzXLR0a53PLpSnu8FQ3VcrC6HBGLg01bsbyGVnLCAM3D3mmpN\nkeGZ0opOkWoEarUXqGSStSxt5hSKlZK8I2j+fFKb7JMzuW7eAFsLapVRHdidMrv4\nMlEHR+cvA1q5CRj3QkvSKdIYUeaq9wDA24ZIR0ppdiCJ2ez6n/uLpRxtT59XjSFw\nvWaumgTCFEIa9VRVelLKP9y6TM0xBSb9Aohb0MogdX6/KGAmX5E4HLw5NKLJHOGM\nyIgH+VeZAgMBAAECggEAAy+lhoc3urgAtbYM76lv4IaZLIrisR5/m1a40rHKtwJA\nRCOKbLpfe8tRyCZ0CZo51E9VcrAQbSEipQSue+9jYacrG0FtDwRR9Mdp4hRzXo4m\nCgpqPl6P+T6XYkpoZ7G03ya2hKnesE8cVkCSxfeD2EEBMoaIadmB9mb1TAlucekB\nK6LhamIheifSkVuTnBStlEPX+0H3aP4W817c0KGG/EdVkoLjfIBeWSP5xMOBSJJT\nN2PJkaNsk7O7ywFkJxrgLbKiGlAfm5jBbbmW9kAQTa9i8/4PbhIF6sPTcelAiEyL\nNJ7mwaVqdUH/A68P8NLpxTptIvTmW4I1u6Ar0iHByQKBgQDEOh4778MnG1milI81\nenbB1nYx+u8MsXxvBlbikAboE3nMVaS+euvNwIO3mAkpT2lW9vQ5UOTpsgmJd/l/\nlQQBzzTQxwgmv5Qtp+BuOD4sCCb2pU0BdXbSGlVPzLyO1648ForzaygyOq/1ujPe\nLbiJND3Njh7xRp9ue/IJwqPI/QKBgQC74ZF4JmZC+NjUvmp1yFPyQPlwheA8Qhz+\nOyZsoC5Cs4/9OESJAOjwI1LIdYqTZf02ENrcRVZCDGIZJxTzi7DcSbhhRmrY+2Mz\nCNZ+PDCxfGZg0c6c+206DMtmpe7z9SQBGu/neUK8x9cRSs8eMmncmBitxx0UJcI6\nQUUrEeqJzQKBgGKOHilUXtwBbJ+vpc3iWEs6/9pSgkYJzsmkkXbxh8aAIahzS28w\nJccNbhqEDfXloK7BEiDHdHG7rfaRf4qIuZ5/B7Pkgz+S8UWND7fMH83Vulwe4fJd\noPQdrcOKvRmxUh1z5Q4lP+caes4cW3i31ftzdacMPpZINkMzlXk5fTGxAoGAKY0j\nfO0RJLKgUbyjEtVxK1yPTgFtrCX6/4bZYqCyWnIX4Cq3jY0z9xf40Pid4ydlLrXf\nkWOMRiMy9tkb2xkDzlRHgMvwCXjfYYQM2/I32qjmg3cjOLiqWXJG8ba0+CM5CT2J\n3SmGRvXzbJGc6NLBctX4b0Zf+fq3z+Zrg7D8q+kCgYBZ7Sx0BzUlR12M/CRLUlvb\nzv8dkNi8N6CBWMbeGkW5IaEEvuurRfESibe/ok2y0rXbRm0iKuEiQPmIqkdhSgE/\nqgs1uAAst/1PukbCVXc7qX3DaiCI/x1xCgsZ7BL3+nlRkcfprkk4oM621O3B1etm\nxMgmrwd56OK6LqeGN5dcvg==\n-----END PRIVATE KEY-----\n"
)

var (
	app           *firebase.App
	opt           option.ClientOption
	client        *storage.Client
	dbConnection  *sql.DB
	projectId     string = "oauthtest-8d82e"
	bucket        *storage.BucketHandle
	dbClient      *firestore.Client
	storageClient *storage.Client
	dberr         error
	storageError  error
)

func Init() {
	// Initialize Firebase app and storage client
	opt = option.WithCredentialsFile("./firebase_credentials.json")
	var err error
	app, err = firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Firebase app initialization error: %v\n", err)
		os.Exit(1)
	}
	dbClient, dberr = firestore.NewClient(context.Background(), projectId, opt)
	if dberr != nil {
		log.Fatalf("Failed to create Firestore client: %v", dberr)
	}
	storageClient, storageError = storage.NewClient(context.Background(), opt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Firebase storage initialization error: %v\n", err)
		os.Exit(1)
	}
	client = storageClient
	bucket = client.Bucket(bucketName)
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
