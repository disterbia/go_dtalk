package handler

import (
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
)

type Video struct {
	Id       string `json:"id"`
	Title    string `firestore:"title" json:"title"`
	Uploader string `firestore:"uploader" json:"uploader"`
	Url      string `firestore:"url" json:"url"`
	//LikeCount   int    `firestore:"like_count" json:"like_count"`
	Upload_time string `firestore:"upload_time" json:"upload_time"`
	Thumbnail   string `firestore:"thumbnail" json:"thumbnail"`
	IsNew       bool   `json:"is_new"`
	//UserLiked   bool   `json:"user_liked"`
	// ChatCount   int      `json:"chat_count"`
	UserInfo UserInfo `json:"user_info"`
}

type UserInfo struct {
	Id        string `firestore:"id" json:"id"`
	Image     string `firestore:"image" json:"image"`
	Thumbnail string `firestore:"thumbnail" json:"thumbnail"`
	Nickname  string `firestore:"nickname" json:"nickname"`
	Intro     string `firestore:"introduction" json:"introduction"`
	// LikeCount      int    `json:"like_count"`
	// FollowerCount  int    `json:"follower_count"`
	// FollowingCount int    `json:"following_count"`
}

func getBlockedVideos(userID string) ([]string, error) {
	var blockedVideos []string

	docs, err := dbClient.Collection("blocklist").Where("userId", "==", userID).Documents(ctx).GetAll()
	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		blockedVideos = append(blockedVideos, doc.Data()["blockedId"].(string))
	}

	return blockedVideos, nil
}

func ReadUserVideos(c *gin.Context) {
	userID := c.DefaultQuery("user_id", "")

	videos, err := getUserVideosFromDatabase(userID)
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

func getUserVideosFromDatabase(userID string) ([]Video, error) {
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
			Id:       doc.Ref.ID,
			Title:    doc.Data()["title"].(string),
			Uploader: doc.Data()["uploader"].(string),
			Url:      doc.Data()["url"].(string),
			//LikeCount:   int(doc.Data()["like_count"].(int64)),
			Upload_time: doc.Data()["upload_time"].(time.Time).Format(time.RFC3339),
			Thumbnail:   doc.Data()["thumbnail"].(string),
			//UserLiked:   userLiked,
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

func ReadVideo(c *gin.Context) {
	pageToken := c.DefaultQuery("pageToken", "0")
	videoStr := c.DefaultQuery("first", "")
	userId := c.DefaultQuery("user_id", "")

	pageSize := 10

	videos, err := getVideosFromDatabase(pageToken, pageSize, videoStr, userId)
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

func getVideosFromDatabase(pageToken string, pageSize int, videoStr string, userId string) ([]Video, error) {
	var videos []Video
	isFirstblock := false
	// Fetch blocked videos
	blockedVideos, err := getBlockedVideos(userId)
	if err != nil {
		return nil, err
	}

	// Get the total count of videos
	// docs, err := dbClient.Collection("videos").OrderBy("upload_time", firestore.Desc).Offset(page).Limit(pageSize).Documents(ctx).GetAll()
	firstdoc, err2 := dbClient.Collection("videos").OrderBy("upload_time", firestore.Desc).Offset(0).Limit(1).Documents(ctx).GetAll()
	if err != nil {
		print(1111)
		return nil, err
	}
	if err2 != nil {
		print(1111)
		return nil, err2
	}

	for _, blockedVideo := range blockedVideos {
		if blockedVideo == firstdoc[0].Ref.ID {
			isFirstblock = true
		}
	}
	if firstdoc[0].Data()["url"].(string) != videoStr && pageToken != "" && !isFirstblock {

		println("first!!")
		userId := firstdoc[0].Data()["uploader"].(string)

		user, err := dbClient.Collection("users").Doc(userId).Get(ctx)
		if err != nil {
			return nil, err
		}

		userInfo := UserInfo{
			Id:        user.Data()["id"].(string),
			Image:     user.Data()["image"].(string),
			Thumbnail: user.Data()["thumbnail"].(string),
			Nickname:  user.Data()["nickname"].(string),
			Intro:     user.Data()["introduction"].(string),
		}
		video := Video{
			Id:          firstdoc[0].Ref.ID,
			Title:       firstdoc[0].Data()["title"].(string),
			Uploader:    firstdoc[0].Data()["uploader"].(string),
			Url:         firstdoc[0].Data()["url"].(string),
			Upload_time: firstdoc[0].Data()["upload_time"].(time.Time).Format(time.RFC3339),
			//LikeCount:   int(firstdoc[0].Data()["like_count"].(int64)),
			IsNew:    true,
			UserInfo: userInfo,
		}

		videos = append(videos, video)
	} else {
		query := dbClient.Collection("videos").OrderBy("upload_time", firestore.Desc).Limit(pageSize)
		if pageToken != "" {
			doc, err := dbClient.Collection("videos").Doc(pageToken).Get(ctx)
			if err != nil {
				return nil, err
			}
			query = query.StartAfter(doc)
		}

		docs, err := query.Documents(ctx).GetAll()
		if err != nil {
			return nil, err
		}

		// Process videos
		for _, doc := range docs {
			creator := doc.Data()["uploader"].(string)

			// Skip the video if the video is in the blocked list
			isBlocked := false
			for _, blockedVideo := range blockedVideos {
				if blockedVideo == doc.Ref.ID {
					isBlocked = true
					break
				}
			}
			if isBlocked {
				continue
			}

			user, err := dbClient.Collection("users").Doc(creator).Get(ctx)
			if err != nil {
				return nil, err
			}

			userInfo := UserInfo{
				Id:        user.Data()["id"].(string),
				Image:     user.Data()["image"].(string),
				Thumbnail: user.Data()["thumbnail"].(string),
				Nickname:  user.Data()["nickname"].(string),
				Intro:     user.Data()["introduction"].(string),
			}

			video := Video{
				Id:          doc.Ref.ID,
				Title:       doc.Data()["title"].(string),
				Uploader:    doc.Data()["uploader"].(string),
				Url:         doc.Data()["url"].(string),
				Upload_time: doc.Data()["upload_time"].(time.Time).Format(time.RFC3339),
				//LikeCount:   int(doc.Data()["like_count"].(int64)),
				IsNew:    false,
				UserInfo: userInfo,
			}

			videos = append(videos, video)
		}
	}

	return videos, nil
}
