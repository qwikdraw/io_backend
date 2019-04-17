package main

import (
	"flag"
	"log"
	"net/http"
	ws "github.com/gorilla/websocket"
	"fmt"
	gp "server/game_protocol"
	proto "github.com/golang/protobuf/proto"
	"encoding/binary"
	"math"
	"time"
)


const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)


type Client struct {
	world *World

	// The websocket connection.
	conn *ws.Conn

	// Buffered channel of outbound messages.
	send chan *ws.PreparedMessage
}

type World struct {
	// Start of server time
	start time.Time

	// Registered clients.
	clients map[*Client]bool

	// Input messages from the clients.
	input chan []byte

	// Outging updates from the server
	update chan []byte

	// Client join
	join chan *Client

	// Client leave
	leave chan *Client
}

func makeWorld() *World {
	return &World{
		start: time.Now(),
		clients: make(map[*Client]bool),
		input: make(chan []byte),
		update: make(chan []byte),
		join: make(chan *Client),
		leave: make(chan *Client),
	}
}

func (world *World) run_networking() {
	for {
		select {
		case client := <-world.join:
			world.clients[client] = true
		case client := <-world.leave:
			if _, exists := world.clients[client]; exists {
				delete(world.clients, client)
				close(client.send)
				client.conn.Close()
				log.Print("client left")
			}
		case message := <-world.input:
			// Handle client input here.
			status := &gp.Status{}
			proto.Unmarshal(message, status)
			log.Print("client: ", status)
		case update := <-world.update:
			msg, err := ws.NewPreparedMessage(ws.BinaryMessage, update)
			if err != nil {
				log.Print("prepare_error:", err)
			}
			for client := range world.clients {
				select {
				case client.send <- msg:
				default:
					close(client.send)
					delete(world.clients, client)
				}
			}
		}
	}
}

func (world *World) run_simulation() {
	for {
		now := time.Now()
		server_time := now.Sub(world.start).Seconds()

		fmt.Println(server_time)
		test_renderable := &gp.Renderable{
			Id: 1,
			Color: 0,
			Size: 0.4, 
		}
		test_entity := &gp.Entity{
			Id: 23,
			PosX: float32(math.Sin(server_time)),
			PosY: -0.4,//float32(math.Cos(server_time)),
			VelX: float32(math.Cos(server_time)),
			VelY: 0.0,//-float32(math.Sin(server_time)),
			Renderable: []*gp.Renderable{test_renderable},
		}
		test_entity_2 := &gp.Entity{
			Id: 24,
			PosX: float32(math.Sin(server_time * 0.3)),
			PosY: 0.4,//float32(math.Cos(server_time)),
			VelX: float32(math.Cos(server_time * 0.3) / 3.0),
			VelY: 0.0,//-float32(math.Sin(server_time)),
			Renderable: []*gp.Renderable{test_renderable},
		}
		update := &gp.Update{
			Time: server_time,
			Entity: []*gp.Entity{test_entity, test_entity_2},
			Remove: []uint32{1, 2},
			CamX: 0.0,
			CamY: float32(math.Cos(server_time)),
			CamScale: 2.0 + float32(math.Cos(server_time)),
		}
		out, err := proto.Marshal(update)
		if err != nil {
			log.Print(err)
		}
		packet := make([]byte, 2, len(out) + 2)
		binary.LittleEndian.PutUint16(packet, uint16(len(out)))
		packet = append(packet, out...)
		world.update <- packet
		// Sleep for 1/30th of a seccond
		time.Sleep(33330000)
	}
}

var addr = flag.String("addr", "0.0.0.0:3000", "http service address")

var upgrader = ws.Upgrader{
	CheckOrigin: func(r *http.Request) bool {return true},
	Subprotocols: []string{"binary"},
}

func (c *Client) readUpdate() {
	defer func() {
		c.world.leave <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil})
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if ws.IsUnexpectedCloseError(err, ws.CloseGoingAway, ws.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
		c.world.input <- message
	}
}

func (c *Client) sendUpdate() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(ws.CloseMessage, []byte{})
				return
			}

			err := c.conn.WritePreparedMessage(msg)
			if err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(ws.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func handleClient(world *World, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade error:", err)
		return
	}

	client := &Client{
		world: world,
		conn: conn,
		send: make(chan *ws.PreparedMessage),
	}
	world.join <- client

	go client.readUpdate()
	go client.sendUpdate()
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	fmt.Printf("Server listening on %s\n", *addr)
	world := makeWorld()
	go world.run_networking()
	go world.run_simulation()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleClient(world, w, r)
	})
	log.Fatal(http.ListenAndServe(*addr, nil))
}
