package main

import (
	"fmt"

	"example.com/gobloc/handler"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()
	router.POST("/multiupload", handler.HandleImageMultiUpload)
	router.GET("/ws", handler.HandleWebSocket)
	router.POST("/upload", handler.VideoHandler)
	fmt.Println("start")
	//router.RunTLS(":443", "./cert.pem", "./key.pem")
	router.Run(":8080")

}
