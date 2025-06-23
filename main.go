package main

import (
	"fmt"
	"math/rand/v2"
	"strconv"

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
	send     chan float64
	sendTime chan int
	sendCrash chan float64
}

var register = make(chan *client)
var gameStartTimerBroadcast = make(chan int)
var startTimer = make(chan int, 2)
var crashedAt = make(chan float64)
var multiplierCount = make(chan float64)
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
		send:     make(chan float64),
		sendTime: make(chan int),
		sendCrash: make(chan float64),
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
		case <-startTimer:
			for {
				fmt.Println("Game starting in:", count)
				if count <= 0 {
					gameStartTimerBroadcast <- 5
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
					crashedAt <- multiplier
					startTimer <- 1
					break
				}
				multiplier += 0.01
				readableMultiplier, err := strconv.ParseFloat(fmt.Sprintf("%.2f", multiplier), 64)
				if err != nil {
					fmt.Println("Error parsing multiplier:", err)
					continue
				}
				multiplierCount <- readableMultiplier
				time.Sleep(100 * time.Millisecond)
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
				cl.send <- multiplier
			}
		case crashed := <-crashedAt:
			for cl := range clients {
				cl.sendCrash <- crashed
			}
		}
	}
}

func writePump(c *client) {
	for {
		select {
		case msg := <-c.send:
			fmt.Println("Sending message to client:", msg, c.conn.RemoteAddr())
			c.conn.WriteJSON(map[string]interface{}{"message": msg, "type": "multiplier"})
		case count := <-c.sendTime:
			c.conn.WriteJSON(map[string]interface{}{"message":  count, "type": "timer"})
			fmt.Println("Sending timer count to client:", count)
		case crash := <-c.sendCrash:
			c.conn.WriteJSON(map[string]interface{}{"message": crash, "type": "crash"})
			fmt.Println("Sending crash info to client:", crash)
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
