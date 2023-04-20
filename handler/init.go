package handler

import (
	"context"
	// "database/sql"
	"fmt"
	"log"
	"os"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
)

var (
	maxWorkers  = 10
	bucketName  = "oauthtest-8d82e.appspot.com"
	imageWidth  = 1024
	imageBucket = "images/"
)

var (
	//app           *firebase.App
	opt    option.ClientOption
	client *storage.Client
	// dbConnection  *sql.DB
	projectId     string = "oauthtest-8d82e"
	bucket        *storage.BucketHandle
	dbClient      *firestore.Client
	storageClient *storage.Client
	dberr         error
	storageError  error
	ctx           = context.Background()
)

func CloseClientsAndConnections() {
	if dbClient != nil {
		dbClient.Close()
	}
	if storageClient != nil {
		storageClient.Close()
	}
	// if dbConnection != nil {
	// 	dbConnection.Close()
	// }
}

func Init() {
	// Initialize Firebase app and storage client
	opt = option.WithCredentialsFile("./firebase_credentials.json")
	// var err error
	// app, err = firebase.NewApp(ctx, nil, opt)
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "Firebase app initialization error: %v\n", err)
	// 	os.Exit(1)
	// }
	dbClient, dberr = firestore.NewClient(ctx, projectId, opt)

	if dberr != nil {
		log.Fatalf("Failed to create Firestore client: %v", dberr)
	}
	storageClient, storageError = storage.NewClient(ctx, opt)

	if storageError != nil {
		fmt.Fprintf(os.Stderr, "Firebase storage initialization error: %v\n", storageError)
		os.Exit(1)
	}
	client = storageClient
	bucket = client.Bucket(bucketName)
	// config := mysql.Config{
	// 	User:                 "root",
	// 	Passwd:               "freedom67",
	// 	Net:                  "tcp",
	// 	Addr:                 "localhost:3306",
	// 	DBName:               "imgurl",
	// 	AllowNativePasswords: true,
	// 	ParseTime:            true,
	// }
	// dbConnection, err := sql.Open("mysql", config.FormatDSN())
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "Database initialization error: %v\n", err)
	// 	os.Exit(1)
	// }

	// // Set up the database connection pool
	// dbConnection.SetMaxIdleConns(10)
	// dbConnection.SetMaxOpenConns(20)
}
