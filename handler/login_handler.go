package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func LoginHandler(c *gin.Context) {
	// 클라이언트에서 전송한 사용자 ID를 가져옵니다.
	userID := c.PostForm("userID")

	// Firestore에서 사용자 ID를 확인합니다.
	_, err := dbClient.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		// 사용자 ID가 없으면 새 사용자를 추가합니다.
		_, err = dbClient.Collection("users").Doc(userID).Set(ctx, map[string]interface{}{
			"id":        userID,
			"image":     "",
			"thumbnail": "",
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": "Registration successful"})
	} else {
		// 사용자 ID가 이미 있으면 로그인 처리를 수행합니다.
		c.JSON(http.StatusOK, gin.H{"message": "Login successful"})
	}
}
