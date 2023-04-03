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
	Conn   *websocket.Conn
	RoomId string
}

type Message struct {
	Username string `json:"username"`
	Text     string `json:"text"`
	RoomId   string `json:"roomId"`
}

type Room struct {
	Users     map[*User]bool
	Broadcast chan Message
}

var rooms = make(map[string]*Room)
var lock = sync.RWMutex{}

func handleMessages(room *Room) {
	for {
		msg := <-room.Broadcast

		lock.RLock()
		for user := range room.Users {
			err := user.Conn.WriteJSON(msg)
			if err != nil {
				fmt.Printf("error: %v", err)
				user.Conn.Close()
				delete(room.Users, user)
			}
		}
		lock.RUnlock()
	}
}

func HandleWebSocket(c *gin.Context) {
	roomId := c.Query("roomId")

	lock.Lock()
	if _, ok := rooms[roomId]; !ok {
		rooms[roomId] = &Room{
			Users:     make(map[*User]bool),
			Broadcast: make(chan Message),
		}
		go handleMessages(rooms[roomId])
	}
	lock.Unlock()

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}

	user := &User{Conn: conn, RoomId: roomId}
	rooms[roomId].Users[user] = true

	message := Message{
		Username: "system",
		Text:     "새로운 사용자가 입장했습니다.",
		RoomId:   roomId,
	}
	rooms[roomId].Broadcast <- message
	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			removeUserFromRoom(roomId, user)
			break
		}

		rooms[roomId].Broadcast <- msg
	}
}

func removeUserFromRoom(roomId string, user *User) {
	lock.Lock()
	defer lock.Unlock()

	if room, ok := rooms[roomId]; ok {
		delete(room.Users, user)
		message := Message{
			Username: "system",
			Text:     "사용자가 퇴장했습니다.",
			RoomId:   roomId,
		}
		room.Broadcast <- message
	}
}
