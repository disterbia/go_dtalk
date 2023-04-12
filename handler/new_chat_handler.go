package handler

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
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
	SendTime string `json:"sendTime"`
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

	// Load chat history
	chatHistory, err := loadChatHistory(roomId)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	} else {
		for _, msg := range chatHistory {
			conn.WriteJSON(msg)
		}
	}

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			removeUserFromRoom(roomId, user)
			break
		}

		// Save new message to Firestore
		saveMessageToFirestore(msg, roomId)

		rooms[roomId].Broadcast <- msg
	}
}

func removeUserFromRoom(roomId string, user *User) {
	lock.Lock()
	defer lock.Unlock()

	if room, ok := rooms[roomId]; ok {
		delete(room.Users, user)
	}
}

func loadChatHistory(roomId string) ([]Message, error) {
	ctx := context.Background()
	messages := []Message{}

	query := dbClient.Collection("chat").Where("roomId", "==", roomId).OrderBy("sendTime", firestore.Asc).Documents(ctx)
	docs, err := query.GetAll()
	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		msg := Message{
			Username: doc.Data()["username"].(string),
			Text:     doc.Data()["text"].(string),
			RoomId:   doc.Data()["roomId"].(string),
			SendTime: doc.Data()["sendTime"].(time.Time).Format(time.RFC3339),
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

func saveMessageToFirestore(msg Message, roomId string) error {
	ctx := context.Background()
	_, _, err := dbClient.Collection("chat").Add(ctx, map[string]interface{}{
		"username": msg.Username,
		"text":     msg.Text,
		"roomId":   roomId,
		"sendTime": time.Now(),
	})

	return err
}
