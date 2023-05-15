package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func LoginHandler(c *gin.Context) {
	// 클라이언트에서 전송한 사용자 ID를 가져옵니다.
	userID := c.PostForm("userID")

	// Firestore에서 사용자 ID를 확인합니다.
	user, err := dbClient.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		// 사용자 ID가 없으면 새 사용자를 추가합니다.
		nickname := "user" + userID[:5]
		_, err = dbClient.Collection("users").Doc(userID).Set(ctx, map[string]interface{}{
			"id":           userID,
			"image":        "",
			"thumbnail":    "",
			"nickname":     nickname,
			"introduction": "hello",
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user"})
			return
		}

		c.JSON(http.StatusOK, UserInfo{
			Id:        userID,
			Image:     "",
			Thumbnail: "",
			Nickname:  nickname,
			Intro:     "hello",
		})
	} else {
		userInfo := UserInfo{
			Id:        user.Data()["id"].(string),
			Image:     user.Data()["image"].(string),
			Nickname:  user.Data()["nickname"].(string),
			Intro:     user.Data()["introduction"].(string),
			Thumbnail: user.Data()["thumbnail"].(string),
		}
		// 사용자 ID가 이미 있으면 로그인 처리를 수행합니다.
		c.JSON(http.StatusOK, userInfo)
	}
}
