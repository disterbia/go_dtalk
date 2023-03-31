package handler

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type User struct {
	Conn *websocket.Conn
}

type Message struct {
	Username string `json:"username"`
	Text     string `json:"text"`
}

var users = make(map[*User]bool)
var broadcast = make(chan Message)
var lock = sync.RWMutex{}

func handleMessages() {
	for {
		msg := <-broadcast

		lock.RLock()
		for user := range users {
			err := user.Conn.WriteJSON(msg)
			if err != nil {
				fmt.Printf("error: %v", err)
				user.Conn.Close()
				delete(users, user)
			}
		}
		lock.RUnlock()
	}
}

func HandleWebSocket(c *gin.Context) {
	go handleMessages()
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	user := &User{Conn: conn}
	users[user] = true

	message := Message{
		Username: "system",
		Text:     "새로운 사용자가 입장했습니다.",
	}
	broadcast <- message

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			delete(users, user)
			break
		}

		broadcast <- msg
	}
}
