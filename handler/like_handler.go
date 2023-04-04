package handler

import (
	"context"
	"errors"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
)

func UpdateLikes(c *gin.Context) {
	// 요청에서 videoID와 like 값을 가져옴
	videoID := c.Param("videoID")
	action := c.PostForm("action")

	// Firestore 클라이언트 초기화
	ctx := context.Background()

	defer dbClient.Close()

	// 좋아요 수 업데이트
	videoRef := dbClient.Collection("videos").Doc(videoID)
	err := dbClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docSnap, err := tx.Get(videoRef)
		if err != nil {
			return err
		}

		var video VideoData
		if err := docSnap.DataTo(&video); err != nil {
			return err
		}

		if action == "like" {
			video.LikesCount++
		} else if action == "dislike" {
			video.LikesCount--
		} else {
			return errors.New("Invalid action parameter")
		}

		tx.Set(videoRef, &video)

		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update likes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Likes updated successfully"})
}
