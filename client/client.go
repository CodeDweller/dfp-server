package client

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// TODO: Implement individual configs for individual clients
type Client struct {
	Ip string
	Conn *websocket.Conn
	IsActive bool
	WriteWait int
	PongWait int
	MaxMessageSize int64
	PingPeriod int
}

// Send Command through websocket
func (c *Client) ClientWriter(sendSignal chan []byte) {
	log.Println("WS writer started:", c.Ip)
	log.Println("Ping ticker duration:", c.PingPeriod)
	pingTicker := time.NewTicker(time.Duration(c.PingPeriod) * time.Second)
	defer func () {
		pingTicker.Stop()
		c.Conn.Close()
	} ()

	for {
		select {
		case message, ok := <-sendSignal:
			c.Conn.SetWriteDeadline(time.Now().Add(time.Duration(c.WriteWait) * time.Second))
			if !ok {
				// If send channel is closed, close websocket
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				log.Println("WS write error in client: ", c.Conn.RemoteAddr())
				log.Println("Unable to send msg through socket: ", err)
				//c.isActive = false
				return
			}
			w.Write(message)
			log.Println("Signal sent!")

			if err := w.Close(); err != nil {
				log.Println("Error closing WS write")
				return
			}

		case <-pingTicker.C:
			// Send ping to client
			log.Println("Sending PING to client")
			c.Conn.SetWriteDeadline(time.Now().Add(time.Duration(c.WriteWait) * time.Second))
			if err := c.Conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				log.Println("Unable to send PING message:", err)
				return
			}
		}
	}

	return
}

// Receive data and maintain WS Connection to the client
func (c *Client) ClientReader (receiveSignal chan []byte, unregSignal chan []byte) {
	defer func () {
		c.Conn.Close()
		log.Println("Sending unregSignal")
		unregSignal <- []byte(c.Ip) // Unregister client after Connection break
	} ()
	log.Println("WS reader started:", c.Ip)

	// Ping-pong
	c.Conn.SetReadLimit(c.MaxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(time.Duration(c.PongWait) * time.Second))
	c.Conn.SetPongHandler(func(string) error {log.Println("Pong received:"); c.Conn.SetReadDeadline(time.Now().Add(time.Duration(c.PongWait) * time.Second)); return nil })

	// Read loop
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			log.Println("WS ReadMessage error: ", err)
			break
		}
		log.Println("Message received: ", string(message))

		receiveSignal <- message
	}
	log.Println("clientReader stopped:", c.Ip)
}