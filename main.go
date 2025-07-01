package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"

	// "math/rand/v2"
	"net/http"
	"time"

	"github.com/google/uuid"
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
	Id             string          `json:"id"`
	Conn           *websocket.Conn `json:"-"` // Skip in JSON, as Conn isn't serializable
	Send           chan float64    `json:"-"` // Skip, channels can't be serialized
	SendTime       chan int        `json:"-"` // Skip
	SendCrash      chan float64    `json:"-"` // Skip
	IsFlying       bool            `json:"isFlying"`
	BetAmount      float64         `json:"betAmount"`
	IsAwaitingBet  bool            `json:"isAwaitingBet"`
	StopMultiplier float64         `json:"stopMultiplier"`
	CanCashOut     bool            `json:"canCashOut"`
}

var register = make(chan *client)
var gameStartTimerBroadcast = make(chan int)
var startTimer = make(chan int, 2)
var crashedAt = make(chan float64)
var multiplierCount = make(chan float64)
var clients []*client
var multiplier float64 = 1.0
var count = 5

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

	c := client{
		Id:        uuid.NewString(),
		Conn:      conn,
		Send:      make(chan float64),
		SendTime:  make(chan int),
		SendCrash: make(chan float64),
		IsFlying:  false,
	}

	// defer func() {
	// 	clients = slices.DeleteFunc(clients, func(item *client) bool {
	// 		fmt.Println("Removing client:", item.conn.RemoteAddr())
	// 		return item.id == c.Id
	// 	})
	// 	c.Conn.Close()
	// }()
	fmt.Println("New client connected:", c.Conn.RemoteAddr())
	register <- &c
	go c.readPump()
	go c.writePump()
}

func handleTimer() {
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
			multiplier = 1.0
			randomMultipierStop := generateRandomMultiplier()
			for {
				if multiplier >= float64(randomMultipierStop) {
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
			for _, cl := range clients {
				cl.SendTime <- count
				fmt.Println("Broadcasting timer count to client:", count)
			}
		case multiplier := <-multiplierCount:
			for _, cl := range clients {
				cl.Send <- multiplier
			}
		case crashed := <-crashedAt:
			for _, cl := range clients {
				cl.SendCrash <- crashed
			}
		}
	}
}

func (c *client) writePump() {
	defer func() {
		c.Conn.Close()
		clients = slices.DeleteFunc(clients, func(cl *client) bool {
			return cl.Id == c.Id
		})
	}()
	for {
		select {
		case msg := <-c.Send:
			fmt.Println("Sending message to client:", msg, c.Conn.RemoteAddr())
			clonedClients := clients[:]
			if len(clonedClients) > 2 {
				slices.SortFunc(clonedClients, func(a, b *client) int {
					return int(a.BetAmount) - int(b.BetAmount)
				})

			}
			jsonClonedClients, err := json.Marshal(clonedClients)
			if err != nil {
				_ = fmt.Errorf("failed to marshal json %v", err)
			}
			if c.CanCashOut {
				err = c.Conn.WriteJSON(map[string]interface{}{"message": msg, "liveClients": string(jsonClonedClients), "type": "multiplier", "canCashOut": true, "betAmount": c.BetAmount})

			} else {
				err = c.Conn.WriteJSON(map[string]interface{}{"message": msg, "liveClients": string(jsonClonedClients), "type": "multiplier", "canCashOut": false, "betAmount": c.BetAmount})

			}
			if err != nil {
				fmt.Println("Error Sending message to client:", err)
				return
			}
		case count := <-c.SendTime:
			err := c.Conn.WriteJSON(map[string]interface{}{"message": count, "type": "timer"})
			if err != nil {
				fmt.Println("Error Sending message to client:", err)
				return
			}
			fmt.Println("Sending timer count to client:", count)
		case crash := <-c.SendCrash:
			// NOTE: this is where the calculation deducting client if he lost by checking if his flying status was false before reaching here
			if c.IsFlying && c.BetAmount != 0{
				fmt.Println("deducting clients fundz")
			}
			c.IsFlying = false
			c.StopMultiplier = 0
			c.CanCashOut = false
			if c.IsAwaitingBet {
				c.IsAwaitingBet = false
				c.IsFlying = true
				c.CanCashOut = true
			} else {
				c.BetAmount = 0

			}
			err := c.Conn.WriteJSON(map[string]interface{}{"message": crash, "type": "crash"})
			if err != nil {
				fmt.Println("Error Sending message to client:", err)
				return
			}
			fmt.Println("Sending crash info to client:", crash)
		}
	}
}

func (c *client) readPump() {
	for {
		var clientMsg struct {
			Intent string  `json:"intent"`
			Amount float64 `json:"amount"`
		}
		err := c.Conn.ReadJSON(&clientMsg)
		if err != nil {
			fmt.Println("Error reading message from client:", err)
			c.Conn.Close()
			break
		}
		for _, cl := range clients {
			if cl.Id == c.Id && cl.Conn.RemoteAddr() == c.Conn.RemoteAddr() {
				if multiplier > 1.0 {
					if clientMsg.Intent == "bet" {
						c.BetAmount = clientMsg.Amount
						c.IsAwaitingBet = true
						c.CanCashOut = false
						fmt.Println("read from client", clientMsg)
						continue
					}
				}
				if multiplier == 1.0 && clientMsg.Intent == "bet" {
					c.BetAmount = clientMsg.Amount
					c.IsAwaitingBet = false
					c.IsFlying = true
					c.CanCashOut = true // NOTE: dont forget to reset on crash or stop
					continue
				}
				if clientMsg.Intent == "stop" && multiplier > 1.0 {
					// NOTE: initiate client reward
					fmt.Println("client rewarded", multiplier*cl.BetAmount)
					cl.IsFlying = false
					c.StopMultiplier = multiplier
				}
			}
		}
	}
}

func handleRegister() {
	for cl := range register {
		fmt.Println("Registering new client:", cl.Conn.RemoteAddr())
		isAlreadyRegistered := slices.ContainsFunc(clients, func(c *client) bool {
			return c.Id == cl.Id && c.Conn.RemoteAddr() == cl.Conn.RemoteAddr()
		})
		if !isAlreadyRegistered {
			appendedCl := append(clients, cl)
			clients = appendedCl
		}
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

// the client would Send message to the server by selecting their multiplier

// the server would broadcast to all clients based on the multiplier they selected to show if they won or lost

// to determine when the client has clicked stop the client would broadcast the message with the client and the message and then the server would let the rest of the clients when the client has clicked stopped and what multiplier they stopped at... the whole thing would be reset after the seconds timer has reached 0
