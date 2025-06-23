package fulldup

import (
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type client struct {
	send chan []byte
	conn *websocket.Conn
}

var register = make(chan *client)
var unregister = make(chan *client)
var clients = make(map[*client]bool)
var broadcast = make(chan []byte)
func mainFul() {
	http.HandleFunc("/game", handleWs)
	go centralHub()
	http.ListenAndServe(":8080", nil)
}

func handleWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Could not upgrade connection", http.StatusInternalServerError)
		return
	}
	cl := &client{conn: conn, send: make(chan []byte)}
	register <- cl
	
	go readPump(cl)
	go writePump(cl)
}

func readPump(cl *client) {
	defer cl.conn.Close()

	for {
		_, msg, err := cl.conn.ReadMessage()
		if err != nil {
			break
		}
		broadcast <- msg
	}
}

func writePump(cl *client) {
	defer func() {
		unregister <- cl
		cl.conn.Close()
	}()

	for msg := range cl.send {
		if err := cl.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}

	}
}

func centralHub() {
	for {
		select {
		case cl := <- register:
			clients[cl] = true
		case msg := <-broadcast:
			for cl := range clients {
				cl.send <- msg
			}
		case cl:= <- unregister:
			delete(clients, cl)
			close(cl.send)
		}
			 
	}
}

