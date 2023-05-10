package handler

import (
	"bufio"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
)

func DeleteVideoHandler(c *gin.Context) {
	docID := c.DefaultQuery("doc_id", "")
	if docID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Document ID is missing"})
		return
	}

	err := deleteVideoFromStorageAndDBByDocID(docID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "Video and associated files successfully deleted"})
}

func deleteVideoFromStorageAndDBByDocID(docID string) error {
	// Get the video document from Firestore using the document ID
	docRef := dbClient.Collection("videos").Doc(docID)
	doc, err := docRef.Get(ctx)
	if err != nil {
		return err
	}

	// Extract uniqueID from the URL
	videoURL := doc.Data()["url"].(string)
	uniqueID := strings.Split(strings.Split(videoURL, "/")[4], "-")[0]

	// Convert uniqueID string to uuid.UUID
	uuid, err := uuid.Parse(uniqueID)
	if err != nil {
		return err
	}

	// Call deleteVideoFromStorageAndDB with the uuid
	return deleteVideoFromStorageAndDB(uuid)
}

func deleteVideoFromStorageAndDB(uniqueID uuid.UUID) error {
	// 1. Download the m3u8 file from Cloud Storage
	m3u8ObjectPath := fmt.Sprintf("videos/%s-*.m3u8", uniqueID)
	m3u8Reader, err := bucket.Object(m3u8ObjectPath).NewReader(ctx)
	if err != nil {
		return err
	}
	defer m3u8Reader.Close()

	// 2. Extract the list of ts files from the m3u8 file
	scanner := bufio.NewScanner(m3u8Reader)
	tsFiles := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasSuffix(line, ".ts") {
			tsFiles = append(tsFiles, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// 3. Delete m3u8 file from Cloud Storage
	if err := bucket.Object(m3u8ObjectPath).Delete(ctx); err != nil {
		return err
	}

	// 4. Delete each ts file from Cloud Storage
	for _, tsURL := range tsFiles {
		objectPath := strings.Split(tsURL, "/")[4]
		if err := bucket.Object(objectPath).Delete(ctx); err != nil {
			return err
		}
	}

	// 5. Delete thumbnail file from Cloud Storage
	thumbnailObjectPath := fmt.Sprintf("thumbnails/%s-thumbnail.webp", uniqueID)
	if err := bucket.Object(thumbnailObjectPath).Delete(ctx); err != nil {
		return err
	}

	// 6. Delete video info from Firestore
	query := dbClient.Collection("videos").Where("url", "==", fmt.Sprintf("https://storage.googleapis.com/%s/%s", bucketName, m3u8ObjectPath))
	iter := query.Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		if _, err := doc.Ref.Delete(ctx); err != nil {
			return err
		}
	}

	return nil
}
