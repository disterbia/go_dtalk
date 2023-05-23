package handler

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type BlockRequest struct {
	UserID    string `json:"userId"`
	BlockedID string `json:"blockId"`
}

func BlcokHandler(c *gin.Context) {
	var req BlockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, _, err := dbClient.Collection("blocklist").Add(ctx, map[string]interface{}{
		"userId":    req.UserID,
		"blockedId": req.BlockedID,
	})
	if err != nil {
		log.Printf("Failed adding document: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to block user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "User blocked successfully"})
}
