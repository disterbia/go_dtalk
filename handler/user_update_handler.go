package handler

import (
	"net/http"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
)

func UpdateUser(c *gin.Context) {
	userId := c.DefaultQuery("user_id", "")
	nickname := c.Request.PostFormValue("nickname")
	intro := c.Request.PostFormValue("intro")

	// Get the user document from Firestore.
	userDocRef := dbClient.Collection("users").Doc(userId)
	userDoc, err := userDocRef.Get(ctx)
	if err != nil {
		c.AbortWithStatus(500)
		return
	}

	if _, ok := userDoc.Data()["nickname"].(string); ok {
		_, err := userDocRef.Update(ctx, []firestore.Update{{Path: "nickname", Value: nickname}})
		if err != nil {
			c.AbortWithStatus(500)
			return
		}
	} else {
		_, err := userDocRef.Set(ctx, map[string]interface{}{
			"nickname": nickname,
		}, firestore.MergeAll)
		if err != nil {
			c.AbortWithStatus(500)
			return
		}
	}

	if _, ok := userDoc.Data()["introduction"].(string); ok {
		_, err := userDocRef.Update(ctx, []firestore.Update{{Path: "introduction", Value: intro}})
		if err != nil {
			c.AbortWithStatus(500)
			return
		}
	} else {
		_, err := userDocRef.Set(ctx, map[string]interface{}{
			"introduction": intro,
		}, firestore.MergeAll)
		if err != nil {
			c.AbortWithStatus(500)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": " updated successfully"})
}
