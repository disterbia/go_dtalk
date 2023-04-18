package handler

import (
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
)

type MyPage struct {
	Id     string  `firestore:"id" json:"id"`
	Image  string  `firestore:"image" json:"image"`
	Videos []Video `json:"videos"`
}

func GetMyPage(c *gin.Context) {
	userID := c.DefaultQuery("user_id", "")

	user, err := getUserFromDatabase(userID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to fetch user videos: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, user)
}

func getUserFromDatabase(userID string) (MyPage, error) {
	var videos []Video
	docs, err := dbClient.Collection("videos").Where("uploader", "==", userID).OrderBy("upload_time", firestore.Desc).Documents(ctx).GetAll()
	if err != nil {
		return MyPage{}, err
	}

	for _, doc := range docs {
		video := Video{
			Title:       doc.Data()["title"].(string),
			Uploader:    doc.Data()["uploader"].(string),
			Url:         doc.Data()["url"].(string),
			LikeCount:   int(doc.Data()["like_count"].(int64)),
			Upload_time: doc.Data()["upload_time"].(time.Time).Format(time.RFC3339),
			Thumbnail:   doc.Data()["thumbnail"].(string),
		}
		videos = append(videos, video)
	}

	user, err2 := dbClient.Collection("users").Doc(userID).Get(ctx)
	if err2 != nil {
		return MyPage{}, err2
	}

	mypage := MyPage{
		Id:     user.Data()["id"].(string),
		Image:  user.Data()["image"].(string),
		Videos: videos,
	}

	return mypage, nil
}
