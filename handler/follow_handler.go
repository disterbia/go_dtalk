package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type FollowInfo struct {
	FollowingCount int64    `json:"following_count"`
	FollowerCount  int64    `json:"follower_count"`
	TotalLikes     int64    `json:"total_likes"`
	UserInfo       UserInfo `json:"user_info"`
}

func GetFollowingUsersInfo(c *gin.Context) {
	userId := c.DefaultQuery("user_id", "")

	followingUsersInfo, err := getFollowingUsersInfo(ctx, userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, followingUsersInfo)
}

func getFollowingUsersInfo(ctx context.Context, userId string) ([]FollowInfo, error) {
	followingsRef := dbClient.Collection("followings").Doc(userId)
	followingsSnapshot, err := followingsRef.Get(ctx)

	var followingIds []interface{}

	if err != nil {
		if status.Code(err) == codes.NotFound {
			followingIds = []interface{}{}
		} else {
			return nil, err
		}
	} else {
		followingIds = followingsSnapshot.Data()["followingIds"].([]interface{})
	}

	followingUsersInfo := make([]FollowInfo, 0)

	for _, followingId := range followingIds {
		followingUserId := followingId.(string)
		followingUserRef := dbClient.Collection("users").Doc(followingUserId)
		followingUserSnapshot, err := followingUserRef.Get(ctx)
		if err != nil {
			return nil, err
		}
		followingUserInfo := followingUserSnapshot.Data()
		userInfo := UserInfo{
			Id:        followingUserSnapshot.Ref.ID,
			Image:     followingUserInfo["image"].(string),
			Thumbnail: followingUserInfo["thumbnail"].(string),
		}

		followersRef := dbClient.Collection("followers").Doc(followingUserId)
		followersSnapshot, err := followersRef.Get(ctx)
		followerCount := int64(0)
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return nil, err
			}
		} else {
			followerCount = int64(len(followersSnapshot.Data()["followerIds"].([]interface{})))
		}

		followingsRef := dbClient.Collection("followings").Doc(followingUserId)
		followingsSnapshot, err := followingsRef.Get(ctx)
		followingCount := int64(0)
		if err != nil {
			if status.Code(err) != codes.NotFound {
				return nil, err
			}
		} else {
			followingCount = int64(len(followingsSnapshot.Data()["followingIds"].([]interface{})))
		}

		videosRef := dbClient.Collection("videos").Where("uploader", "==", followingUserId)
		videosSnapshot, err := videosRef.Documents(ctx).GetAll()
		if err != nil {
			return nil, err
		}
		totalLikes := int64(0)
		for _, doc := range videosSnapshot {
			likeCount := doc.Data()["like_count"].(int64)
			totalLikes += likeCount
		}

		followInfo := FollowInfo{
			FollowingCount: followingCount,
			FollowerCount:  followerCount,
			TotalLikes:     totalLikes,
			UserInfo:       userInfo,
		}

		followingUsersInfo = append(followingUsersInfo, followInfo)
	}

	return followingUsersInfo, nil
}

// func ToggleFollow(c *gin.Context) {
// 	followerId := c.PostForm("user_id") // PostForm 메소드를 사용하여 폼 데이터에서 userId를 추출합니다.
// 	followingId := c.PostForm("creator")

// 	if followerId == followingId {
// 		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot follow yourself"})
// 		return
// 	}

// 	// 팔로우 상태 확인
// 	isFollowing, err := isUserFollowing(ctx, followerId, followingId)
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}

// 	if isFollowing {
// 		// 언팔로우 처리
// 		err = unfollow(ctx, followerId, followingId)
// 	} else {
// 		// 팔로우 처리
// 		err = follow(ctx, followerId, followingId)
// 	}

// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}

// 	c.JSON(http.StatusOK, gin.H{"status": "success"})
// }

// func isUserFollowing(ctx context.Context, followerId, followingId string) (bool, error) {
// 	doc, err := dbClient.Collection("followings").Doc(followerId).Get(ctx)
// 	if err != nil {
// 		return false, err
// 	}

// 	data := doc.Data()
// 	followingIds, ok := data["followingIds"].([]interface{})
// 	if !ok {
// 		return false, nil
// 	}

// 	for _, id := range followingIds {
// 		if id == followingId {
// 			return true, nil
// 		}
// 	}

// 	return false, nil
// }
// func follow(ctx context.Context, followerId, followingId string) error {
// 	// 팔로워 목록에 팔로우하는 사람 추가
// 	_, err := dbClient.Collection("followers").Doc(followingId).Set(ctx, map[string]interface{}{
// 		"followerIds": firestore.ArrayUnion(followerId),
// 	}, firestore.MergeAll)

// 	if err != nil {
// 		return err
// 	}

// 	// 팔로잉 목록에 팔로우 대상 추가
// 	_, err = dbClient.Collection("followings").Doc(followerId).Set(ctx, map[string]interface{}{
// 		"followingIds": firestore.ArrayUnion(followingId),
// 	}, firestore.MergeAll)

// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// func unfollow(ctx context.Context, followerId, followingId string) error {
// 	// 팔로워 목록에서 팔로우하는 사람 제거
// 	_, err := dbClient.Collection("followers").Doc(followingId).Set(ctx, map[string]interface{}{
// 		"followerIds": firestore.ArrayRemove(followerId),
// 	}, firestore.MergeAll)

// 	if err != nil {
// 		return err
// 	}

// 	// 팔로잉 목록에서 팔로우 대상 제거
// 	_, err = dbClient.Collection("followings").Doc(followerId).Set(ctx, map[string]interface{}{
// 		"followingIds": firestore.ArrayRemove(followingId),
// 	}, firestore.MergeAll)

// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }
