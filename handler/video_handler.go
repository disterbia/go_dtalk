package handler

import (
	"fmt"

	"net/http"

	"strconv"

	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
)

type Video struct {
	Id          string `json:"id"`
	Title       string `firestore:"title" json:"title"`
	Uploader    string `firestore:"uploader" json:"uploader"`
	Url         string `firestore:"url" json:"url"`
	LikeCount   int    `firestore:"like_count" json:"like_count"`
	Upload_time string `firestore:"upload_time" json:"upload_time"`
	Thumbnail   string `firestore:"thumbnail" json:"thumbnail"`
	IsNew       bool   `json:"is_new"`
	// UserLiked   bool     `json:"user_liked"`
	// ChatCount   int      `json:"chat_count"`
	UserInfo UserInfo `json:"user_info"`
}

type UserInfo struct {
	Id    string `firestore:"id" json:"id"`
	Image string `firestore:"image" json:"image"`
}

func ReadUserVideos(c *gin.Context) {
	userID := c.DefaultQuery("user_id", "")
	requestingUserID := c.DefaultQuery("requesting_user_id", "")

	videos, err := getUserVideosFromDatabase(userID, requestingUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to fetch user videos: %v", err),
		})
		return
	}

	if videos == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "No videos found",
		})
		return
	}

	c.JSON(http.StatusOK, videos)
}

func getUserVideosFromDatabase(userID string, requestingUserID string) ([]Video, error) {
	var videos []Video
	docs, err := dbClient.Collection("videos").Where("uploader", "==", userID).OrderBy("upload_time", firestore.Desc).Documents(ctx).GetAll()

	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		// videoID := doc.Ref.ID
		// userLiked, err := checkUserLikedVideo(requestingUserID, videoID)
		// chatCount, err2 := checkChatCount(doc.Data()["url"].(string))

		// if err != nil {
		// 	return nil, err
		// }
		// if err2 != nil {
		// 	return nil, err2
		// }

		video := Video{
			Id:          doc.Ref.ID,
			Title:       doc.Data()["title"].(string),
			Uploader:    doc.Data()["uploader"].(string),
			Url:         doc.Data()["url"].(string),
			LikeCount:   int(doc.Data()["like_count"].(int64)),
			Upload_time: doc.Data()["upload_time"].(time.Time).Format(time.RFC3339),
			Thumbnail:   doc.Data()["thumbnail"].(string),
			// UserLiked:   userLiked,
			// ChatCount:   chatCount,
		}
		videos = append(videos, video)
	}

	return videos, nil
}

// func checkChatCount(videoUrl string) (int, error) {
// 	chats, err3 := dbClient.Collection("chat").Where("roomId", "==", videoUrl).Documents(ctx).GetAll()
// 	if err3 != nil {
// 		return 0, err3
// 	}
// 	chatCount := len(chats)
// 	return chatCount, nil
// }

// func checkUserLikedVideo(userID string, videoID string) (bool, error) {

// 	if userID == "" {
// 		return false, nil
// 	}

// 	userLikeDoc, err := dbClient.Collection("user_likes").Doc(userID).Get(ctx)
// 	if err != nil {
// 		if status.Code(err) == codes.NotFound {
// 			return false, nil
// 		}
// 		return false, err
// 	}

// 	likedVideos := userLikeDoc.Data()["liked_videos"].(map[string]bool)
// 	_, liked := likedVideos[videoID]

// 	return liked, nil
// }

func ReadVideo(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "0")
	videoStr := c.DefaultQuery("first", "")
	userId := c.DefaultQuery("user_id", "")
	page, err := strconv.Atoi(pageStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid page number",
		})
		return
	}

	pageSize := 1
	if page == 0 {
		pageSize = 3
	}

	videos, err := getVideosFromDatabase(page, pageSize, videoStr, userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to fetch videos",
		})
		return
	}

	if videos == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Page not found",
		})
		return
	}

	c.JSON(http.StatusOK, videos)
}

func getVideosFromDatabase(page int, pageSize int, videoStr string, userId string) ([]Video, error) {
	var videos []Video

	// Get the total count of videos
	docs, err := dbClient.Collection("videos").OrderBy("upload_time", firestore.Desc).Offset(page).Limit(pageSize).Documents(ctx).GetAll()
	firstdoc, err2 := dbClient.Collection("videos").OrderBy("upload_time", firestore.Desc).Offset(0).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		print(1111)
		return nil, err
	}
	if err2 != nil {
		print(2222)
		return nil, err2
	}

	totalCount := len(docs)

	// Calculate the number of random documents needed
	if pageSize > totalCount {
		print(3333)
		return nil, nil
	}

	if firstdoc[0].Data()["url"].(string) != videoStr && page != 0 {

		userId := firstdoc[0].Data()["uploader"].(string)
		user, err := dbClient.Collection("users").Doc(userId).Get(ctx)
		if err != nil {
			return nil, err
		}

		userInfo := UserInfo{
			Id:    user.Data()["id"].(string),
			Image: user.Data()["image"].(string),
		}
		video := Video{
			Id:          firstdoc[0].Ref.ID,
			Title:       firstdoc[0].Data()["title"].(string),
			Uploader:    firstdoc[0].Data()["uploader"].(string),
			Url:         firstdoc[0].Data()["url"].(string),
			Upload_time: firstdoc[0].Data()["upload_time"].(time.Time).Format(time.RFC3339),
			LikeCount:   int(firstdoc[0].Data()["like_count"].(int64)),
			IsNew:       true,
			UserInfo:    userInfo,
		}

		// videoLiked, err := checkUserLikedVideo(userId, firstdoc[0].Ref.ID)
		// chatCount, err2 := checkChatCount(firstdoc[0].Data()["url"].(string))

		// if err != nil {
		// 	return nil, err
		// }
		// if err2 != nil {
		// 	return nil, err2
		// }

		// video.UserLiked = videoLiked
		// video.ChatCount = chatCount
		videos = append(videos, video)

	} else {
		for i := 0; i < pageSize; i++ {
			doc := docs[i]
			userId := doc.Data()["uploader"].(string)
			user, err := dbClient.Collection("users").Doc(userId).Get(ctx)
			if err != nil {
				return nil, err
			}

			userInfo := UserInfo{
				Id:    user.Data()["id"].(string),
				Image: user.Data()["image"].(string),
			}
			video := Video{
				Id:          doc.Ref.ID,
				Title:       doc.Data()["title"].(string),
				Uploader:    doc.Data()["uploader"].(string),
				Url:         doc.Data()["url"].(string),
				LikeCount:   int(doc.Data()["like_count"].(int64)),
				Upload_time: doc.Data()["upload_time"].(time.Time).Format(time.RFC3339),
				IsNew:       false,
				UserInfo:    userInfo,
			}
			// videoLiked, err := checkUserLikedVideo(userId, doc.Ref.ID)
			// if err != nil {
			// 	return nil, err
			// }
			// chatCount, err2 := checkChatCount(doc.Data()["url"].(string))
			// if err2 != nil {
			// 	return nil, err2
			// }
			// video.UserLiked = videoLiked
			// video.ChatCount = chatCount
			videos = append(videos, video)
		}
	}

	return videos, nil
}
