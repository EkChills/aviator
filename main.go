package main

import (
	"fmt"
	"math/rand/v2"
	// "math/rand/v2"
	"net/http"
	"time"

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
	conn     *websocket.Conn
	send     chan []byte
	sendTime chan int
}

var timerDoneChan = make(chan string, 2)
var register = make(chan *client)
var gameStartTimerBroadcast = make(chan int)
var startTimer = make(chan int)
var multiplierCount = make(chan string)
var clients = make(map[*client]bool)

func main() {
	go handleRegister()
	go handleTimer()
	go timerLogger()
	http.HandleFunc("/game", handleWs)
	http.ListenAndServe(":8080", nil)
}

func handleWs(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Could not upgrade connection", http.StatusInternalServerError)
		return
	}

	c := &client{
		conn:     conn,
		send:     make(chan []byte),
		sendTime: make(chan int),
	}

	fmt.Println("New client connected:", c.conn.RemoteAddr())
	register <- c
	go writePump(c)
}

func handleTimer() {
	count := 5
	for {
		count = 5
		fmt.Println("Waiting for timer to start...")
		select {
		case <-timerDoneChan:
			fmt.Println("Timer done, resetting count.")

			count = 5
		case <-startTimer:
			for {
				fmt.Println("Game starting in:", count)
				if count <= 0 {
					gameStartTimerBroadcast <- 5
					timerDoneChan <- "done"
					break
				}
				count--
				gameStartTimerBroadcast <- count
				time.Sleep(1 * time.Second)
			}
		default:
			var multiplier float64 = 1.0
			randomMultipierStop := generateRandomMultiplier()
			for{
				if(multiplier >= float64(randomMultipierStop)) {
					break
				}
				multiplier += 0.1
				readableMultiplier := fmt.Sprintf("%.2f", multiplier)
				multiplierCount <- readableMultiplier
				time.Sleep(400 * time.Millisecond)
			}
		}
	}

}

func timerLogger() {
	for {
		select {
		case count := <-gameStartTimerBroadcast:
			for cl := range clients {
				cl.sendTime <- count
				fmt.Println("Broadcasting timer count to client:", count)
			}
		case multiplier := <-multiplierCount:
			for cl := range clients {
				cl.send <- []byte(multiplier)
			}
		}
	}
}

func writePump(c *client) {
	for {
		select {
		case msg := <-c.send:
			fmt.Println("Sending message to client:", string(msg))
		case count := <-c.sendTime:
			c.conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Timer count: %d", count)))
			fmt.Println("Sending timer count to client:", count)
		}
	}
}

func handleRegister() {
	for cl := range register {
		fmt.Println("Registering new client:", cl.conn.RemoteAddr())
		clients[cl] = true
	}
}

func generateRandomMultiplier() int {
	randomNo := rand.IntN(10)
	if randomNo < 1 {
		return 1
	}
	return randomNo
}

//AviatorBackendFullDup//

// every client would connect to the server and wait for message events from the server(this would be the currently increasing multiplier). NOTE: there would be like a timer that can reset when the clients result have been sent back in the stream so only after the deadline of that timer the multiplier starts

// the client would send message to the server by selecting their multiplier

// the server would broadcast to all clients based on the multiplier they selected to show if they won or lost
