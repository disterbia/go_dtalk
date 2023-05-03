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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Event struct {
	EventType    string     `json:"event_type"`
	Message      *Message   `json:"message,omitempty"`
	FirstMessage *[]Message `json:"first_message,omitempty"`
	TotalLike    *int       `json:"total_like,omitempty"`
	UserLike     *bool      `json:"user_like,omitempty"`
	UserId       *string    `json:"user_id,omitempty"`
}

type User struct {
	Conn   *websocket.Conn
	RoomId string
}

type Message struct {
	Username   string `json:"username"`
	Text       string `json:"text"`
	RoomId     string `json:"room_id"`
	TotalCount int    `json:"total_count"`
	SendTime   string `json:"sendTime"`
}

type Room struct {
	Users     map[*User]bool
	Broadcast chan Event
}

var rooms = make(map[string]*Room)
var lock = sync.RWMutex{}

func handleEvents(room *Room) {
	for {
		event := <-room.Broadcast

		lock.RLock()
		for user := range room.Users {
			err := user.Conn.WriteJSON(event)
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
	roomId := c.Query("room_id")
	userId := c.Query("user_id")

	lock.Lock()
	if _, ok := rooms[roomId]; !ok {
		rooms[roomId] = &Room{
			Users:     make(map[*User]bool),
			Broadcast: make(chan Event),
		}
		go handleEvents(rooms[roomId])
	}
	lock.Unlock()

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Printf("egrror: %v\n", err)
		return
	}

	user := &User{Conn: conn, RoomId: roomId}
	rooms[roomId].Users[user] = true

	// Load chat history
	chatHistory, err := loadChatHistory(roomId)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	} else {
		event := Event{
			EventType: "first_message",
			// Message:   nil,
			FirstMessage: nil,
		}
		event.FirstMessage = &chatHistory
		conn.WriteJSON(event)
		println("message:", len(chatHistory), "roomId", roomId)
		// for _, msg := range chatHistory {
		// 	event.Message = &msg
		// 	conn.WriteJSON(event)
		// 	println("message:", len(chatHistory), "roomId", roomId)
		// }
	}

	// Send total likes
	totalLikes, err := getTotalLikes(roomId)
	userLiked, err2 := checkUserLikedVideo(userId, roomId)
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	if err2 != nil {
		fmt.Printf("error: %v\n", err2)
	}

	event := Event{
		EventType: "first_like",
		TotalLike: &totalLikes,
		UserLike:  &userLiked,
		UserId:    &userId,
	}

	conn.WriteJSON(event)
	println("first_lLike:", totalLikes, userLiked, len(chatHistory), roomId)

	for {
		var event Event
		err := conn.ReadJSON(&event)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			removeUserFromRoom(roomId, user)
			break
		}

		switch event.EventType {
		case "message":
			if event.Message != nil {
				// Save new message to Firestore
				saveMessageToFirestore(*event.Message, roomId)

				// Update total count
				roomDocCount, err := getMessageDocCount(roomId)
				if err != nil {
					fmt.Printf("error: %v\n", err)
				} else {
					event.Message.TotalCount = roomDocCount
				}
				event.Message.SendTime = time.Now().Format(time.RFC3339)
				rooms[roomId].Broadcast <- event
			}
		case "like":
			userId := event.UserId
			likeEvent, err := handleLikeEvent(*userId, roomId)
			if err != nil {
				fmt.Printf("error: %v\n", err)
			} else {
				println(&likeEvent.TotalLike)
				rooms[roomId].Broadcast <- *likeEvent
			}
		}
	}
}

func checkUserLikedVideo(userID string, videoID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	userLikeDoc, err := dbClient.Collection("user_likes").Doc(userID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Create a new document with the specified user ID
			_, err := dbClient.Collection("user_likes").Doc(userID).Set(ctx, map[string]interface{}{
				"like_videos": map[string]bool{},
			})
			if err != nil {
				return false, err
			}
			return false, nil
		}
		return false, err
	}
	likedVideos := userLikeDoc.Data()["like_videos"].(map[string]interface{})
	liked, ok := likedVideos[videoID].(bool)
	if !ok {

		liked = false
	}
	return liked, nil
}

func removeUserFromRoom(roomId string, user *User) {
	lock.Lock()
	defer lock.Unlock()

	if room, ok := rooms[roomId]; ok {
		delete(room.Users, user)
	}
}

func loadChatHistory(roomId string) ([]Message, error) {
	messages := []Message{}

	query := dbClient.Collection("chat").Where("roomId", "==", roomId).OrderBy("sendTime", firestore.Desc).Documents(ctx)
	docs, err := query.GetAll()
	if err != nil {
		return nil, err
	}

	for _, doc := range docs {
		msg := Message{
			Username:   doc.Data()["username"].(string),
			Text:       doc.Data()["text"].(string),
			RoomId:     doc.Data()["roomId"].(string),
			TotalCount: len(docs),
			SendTime:   doc.Data()["sendTime"].(time.Time).Format(time.RFC3339),
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

func saveMessageToFirestore(msg Message, roomId string) error {
	_, _, err := dbClient.Collection("chat").Add(ctx, map[string]interface{}{
		"username": msg.Username,
		"text":     msg.Text,
		"roomId":   roomId,
		"sendTime": time.Now(),
	})

	return err
}

func getTotalLikes(roomId string) (int, error) {
	doc, err := dbClient.Collection("videos").Doc(roomId).Get(ctx)
	if err != nil {
		return 0, err
	}

	totalLikes := doc.Data()["like_count"].(int64)
	return int(totalLikes), nil
}

func getMessageDocCount(roomId string) (int, error) {
	query := dbClient.Collection("chat").Where("roomId", "==", roomId)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return 0, err
	}

	return len(docs), nil
}

func convertToMapStringBool(inputMap map[string]interface{}) map[string]bool {
	outputMap := make(map[string]bool)
	for key, value := range inputMap {
		if boolValue, ok := value.(bool); ok {
			outputMap[key] = boolValue
		}
	}
	return outputMap
}
func handleLikeEvent(userId string, roomId string) (*Event, error) {
	temp := false

	userDocRef := dbClient.Collection("user_likes").Doc(userId)
	videoDocRef := dbClient.Collection("videos").Doc(roomId)

	userDoc, err := userDocRef.Get(ctx)
	if err != nil {
		return nil, err
	}

	likedVideosData := userDoc.Data()["like_videos"]
	var likedVideos map[string]bool
	if likedVideosData != nil {
		likedVideos = convertToMapStringBool(likedVideosData.(map[string]interface{}))
	} else {
		likedVideos = make(map[string]bool)
	}
	// Like or unlike the video
	if _, ok := likedVideos[roomId]; !ok {
		likedVideos[roomId] = true

		err = dbClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			videoDoc, err := tx.Get(videoDocRef)
			if err != nil {
				return err
			}

			likeCount := videoDoc.Data()["like_count"].(int64)
			return tx.Update(videoDocRef, []firestore.Update{
				{Path: "like_count", Value: likeCount + 1},
			})
		})
		if err != nil {
			return nil, err
		}
		temp = true
	} else {
		delete(likedVideos, roomId)

		err = dbClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
			videoDoc, err := tx.Get(videoDocRef)
			if err != nil {
				return err
			}

			likeCount := videoDoc.Data()["like_count"].(int64)
			return tx.Update(videoDocRef, []firestore.Update{
				{Path: "like_count", Value: likeCount - 1},
			})
		})
		if err != nil {
			return nil, err
		}
		temp = false
	}

	// Update user's liked videos
	_, err = userDocRef.Update(ctx, []firestore.Update{
		{Path: "like_videos", Value: likedVideos},
	})
	if err != nil {
		return nil, err
	}

	// Get updated like count
	totalLikes, err := getTotalLikes(roomId)
	if err != nil {
		return nil, err
	}

	return &Event{
		EventType: "total_like",
		TotalLike: &totalLikes,
		UserLike:  &temp,
		UserId:    &userId,
	}, nil
}

// package handler

// import (
// 	"context"
// 	"fmt"
// 	"net/http"
// 	"sync"
// 	"time"

// 	"cloud.google.com/go/firestore"
// 	"github.com/gin-gonic/gin"
// 	"github.com/gorilla/websocket"
// )

// var upgrader = websocket.Upgrader{
// 	ReadBufferSize:  1024,
// 	WriteBufferSize: 1024,
// 	CheckOrigin: func(r *http.Request) bool {
// 		return true
// 	},
// }

// type Event struct {
// 	EventType string   `json:"event_type"`
// 	Message     *Message `json:"message,omitempty"`
// 	TotalLike   int      `json:"total_like,omitempty"`
// }
// type User struct {
// 	Conn   *websocket.Conn
// 	RoomId string
// }

// type Message struct {
// 	Username   string `json:"username"`
// 	Text       string `json:"text"`
// 	RoomId     string `json:"roomId"`
// 	TotalCount int    `json:"total_count"`
// 	SendTime   string `json:"sendTime"`
// }

// type Room struct {
// 	Users     map[*User]bool
// 	Broadcast chan Message
// }

// var rooms = make(map[string]*Room)
// var lock = sync.RWMutex{}

// func handleMessages(room *Room) {
// 	for {
// 		msg := <-room.Broadcast

// 		lock.RLock()
// 		for user := range room.Users {
// 			err := user.Conn.WriteJSON(msg)
// 			if err != nil {
// 				fmt.Printf("error: %v", err)
// 				user.Conn.Close()
// 				delete(room.Users, user)
// 			}
// 		}
// 		lock.RUnlock()
// 	}
// }

// func HandleWebSocket(c *gin.Context) {
// 	roomId := c.Query("roomId")

// 	lock.Lock()
// 	if _, ok := rooms[roomId]; !ok {
// 		rooms[roomId] = &Room{
// 			Users:     make(map[*User]bool),
// 			Broadcast: make(chan Message),
// 		}
// 		go handleMessages(rooms[roomId])
// 	}
// 	lock.Unlock()

// 	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
// 	if err != nil {
// 		fmt.Printf("error: %v\n", err)
// 		return
// 	}

// 	user := &User{Conn: conn, RoomId: roomId}
// 	rooms[roomId].Users[user] = true

// 	// Load chat history
// 	chatHistory, err := loadChatHistory(roomId)
// 	if err != nil {
// 		fmt.Printf("error: %v\n", err)
// 	} else {
// 		for _, msg := range chatHistory {
// 			conn.WriteJSON(msg)
// 		}
// 	}

// 	for {
// 		var msg Message
// 		err := conn.ReadJSON(&msg)
// 		if err != nil {
// 			fmt.Printf("error: %v\n", err)
// 			removeUserFromRoom(roomId, user)
// 			break
// 		}

// 		// Save new message to Firestore
// 		saveMessageToFirestore(msg, roomId)

// 		rooms[roomId].Broadcast <- msg
// 	}
// }

// func removeUserFromRoom(roomId string, user *User) {
// 	lock.Lock()
// 	defer lock.Unlock()

// 	if room, ok := rooms[roomId]; ok {
// 		delete(room.Users, user)
// 	}
// }

// func loadChatHistory(roomId string) ([]Message, error) {
// 	ctx := context.Background()
// 	messages := []Message{}

// 	query := dbClient.Collection("chat").Where("roomId", "==", roomId).OrderBy("sendTime", firestore.Desc).Documents(ctx)
// 	docs, err := query.GetAll()
// 	if err != nil {
// 		return nil, err
// 	}

// 	for _, doc := range docs {
// 		msg := Message{
// 			Username: doc.Data()["username"].(string),
// 			Text:     doc.Data()["text"].(string),
// 			RoomId:   doc.Data()["roomId"].(string),
// 			SendTime: doc.Data()["sendTime"].(time.Time).Format(time.RFC3339),
// 		}
// 		messages = append(messages, msg)
// 	}

// 	return messages, nil
// }

// func saveMessageToFirestore(msg Message, roomId string) error {
// 	ctx := context.Background()
// 	_, _, err := dbClient.Collection("chat").Add(ctx, map[string]interface{}{
// 		"username": msg.Username,
// 		"text":     msg.Text,
// 		"roomId":   roomId,
// 		"sendTime": time.Now(),
// 	})

// 	return err
// }
