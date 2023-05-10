package main

import (
	"fmt"

	"example.com/gobloc/handler"

	"github.com/gin-gonic/gin"
)

func main() {
	handler.Init()
	router := gin.Default()
	router.MaxMultipartMemory = 8 << 20 // 8 MiB
	router.POST("/multiupload", handler.HandleImageMultiUpload)
	router.GET("/ws", handler.HandleWebSocket)
	router.GET("/videos", handler.ReadVideo)
	router.GET("/mypage", handler.GetMyPage)
	router.GET("/user_videos", handler.ReadUserVideos)
	router.POST("/uploads", handler.VideoObjectHandler)
	router.POST("/login", handler.LoginHandler)
	router.GET("/follow", handler.GetFollowingUsersInfo)
	router.POST("/delete", handler.DeleteVideo)
	fmt.Println("start")
	//router.RunTLS(":443", "./cert.pem", "./key.pem")
	router.Run(":8080")
	defer handler.CloseClientsAndConnections()

}
