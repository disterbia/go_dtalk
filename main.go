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
	router.POST("/upload", handler.VideoHandler)
	router.POST("/uploads", handler.VideoObjectHandler)
	router.POST("/login", handler.LoginHandler)
	fmt.Println("start")
	//router.RunTLS(":443", "./cert.pem", "./key.pem")
	router.Run(":8080")
	defer handler.CloseClientsAndConnections()

}
