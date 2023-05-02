package handler

import (
	"context"
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
)

func ToggleFollow(c *gin.Context) {
	followerId := c.PostForm("user_id") // PostForm 메소드를 사용하여 폼 데이터에서 userId를 추출합니다.
	followingId := c.PostForm("creator")

	if followerId == followingId {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot follow yourself"})
		return
	}

	// 팔로우 상태 확인
	isFollowing, err := isUserFollowing(ctx, followerId, followingId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if isFollowing {
		// 언팔로우 처리
		err = unfollow(ctx, followerId, followingId)
	} else {
		// 팔로우 처리
		err = follow(ctx, followerId, followingId)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success"})
}

func isUserFollowing(ctx context.Context, followerId, followingId string) (bool, error) {
	doc, err := dbClient.Collection("followings").Doc(followerId).Get(ctx)
	if err != nil {
		return false, err
	}

	data := doc.Data()
	followingIds, ok := data["followingIds"].([]interface{})
	if !ok {
		return false, nil
	}

	for _, id := range followingIds {
		if id == followingId {
			return true, nil
		}
	}

	return false, nil
}

func GetFollowerInfo(c *gin.Context) {
	userId := c.PostForm("userId")
	creatorId := c.PostForm("creatorId")

	following, err := isUserFollowing(ctx, userId, creatorId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	doc, err := dbClient.Collection("followers").Doc(creatorId).Get(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	data := doc.Data()
	followerIds, ok := data["followerIds"].([]interface{})
	if !ok {
		c.JSON(http.StatusOK, gin.H{"count": 0, "following": following})
		return
	}

	c.JSON(http.StatusOK, gin.H{"count": len(followerIds), "following": following})
}

func follow(ctx context.Context, followerId, followingId string) error {
	// 팔로워 목록에 팔로우하는 사람 추가
	_, err := dbClient.Collection("followers").Doc(followingId).Set(ctx, map[string]interface{}{
		"followerIds": firestore.ArrayUnion(followerId),
	}, firestore.MergeAll)

	if err != nil {
		return err
	}

	// 팔로잉 목록에 팔로우 대상 추가
	_, err = dbClient.Collection("followings").Doc(followerId).Set(ctx, map[string]interface{}{
		"followingIds": firestore.ArrayUnion(followingId),
	}, firestore.MergeAll)

	if err != nil {
		return err
	}

	return nil
}

func unfollow(ctx context.Context, followerId, followingId string) error {
	// 팔로워 목록에서 팔로우하는 사람 제거
	_, err := dbClient.Collection("followers").Doc(followingId).Set(ctx, map[string]interface{}{
		"followerIds": firestore.ArrayRemove(followerId),
	}, firestore.MergeAll)

	if err != nil {
		return err
	}

	// 팔로잉 목록에서 팔로우 대상 제거
	_, err = dbClient.Collection("followings").Doc(followerId).Set(ctx, map[string]interface{}{
		"followingIds": firestore.ArrayRemove(followingId),
	}, firestore.MergeAll)

	if err != nil {
		return err
	}

	return nil
}
