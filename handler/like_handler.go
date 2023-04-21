package handler

import (
	"context"
	"net/http"
	"strconv"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func UpdateLikes(c *gin.Context) {
	userID := c.Param("userID")
	videoID := c.Param("videoID")
	action, err := strconv.Atoi(c.PostForm("action"))

	if err != nil || (action != 1 && action != -1) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid action parameter"})
		return
	}

	// user_likes 컬렉션 참조
	userLikesRef := dbClient.Collection("user_likes").Doc(userID)
	videoRef := dbClient.Collection("videos").Doc(videoID)

	err = dbClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// user_likes 도큐먼트 가져오기
		userLikesDoc, err := tx.Get(userLikesRef)
		if err != nil && status.Code(err) != codes.NotFound {
			return err
		}

		var userLikes map[string]bool
		if userLikesDoc.Exists() {
			if err := userLikesDoc.DataTo(&userLikes); err != nil {
				return err
			}
		} else {
			userLikes = make(map[string]bool)
		}

		if action == 1 {
			userLikes[videoID] = true
		} else {
			delete(userLikes, videoID)
		}

		tx.Set(userLikesRef, userLikes)

		// 좋아요 수 업데이트
		updateData := []firestore.Update{
			{
				Path:  "LikeCount",
				Value: firestore.Increment(action),
			},
		}
		err = tx.Update(videoRef, updateData)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update likes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Likes updated successfully"})
}
