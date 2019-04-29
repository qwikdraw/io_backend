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
	"github.com/ByteArena/box2d"
	"sync"
	"sync/atomic"
)


const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 10 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 20
)


type Entity interface {
	Id() uint32
	Type() uint32
	Update(float64)
	Renderables() []*gp.Renderable
	PosX() float64
	PosY() float64
	VelX() float64
	VelY() float64
	DirX() float64
	DirY() float64
}

type Player struct {
	id uint32
	Input chan *gp.Status
	Body *box2d.B2Body
	renderables []*gp.Renderable
}

func (p *Player) Id() uint32 {
	return p.id
}

func (p *Player) Type() uint32 {
	return 1
}

func (p *Player) Update(dt float64) {
	select {
		case in := <- p.Input:
			imp := box2d.B2Vec2{
				X: float64(in.VelX),
				Y: float64(in.VelY),
			}
			p.Body.SetLinearVelocity(imp)
	}
}

func (p *Player) Renderables() []*gp.Renderable {
	return p.renderables
}

func (p *Player) PosX() float64 {
	return p.Body.GetPosition().X
}

func (p *Player) PosY() float64 {
	return p.Body.GetPosition().Y
}

func (p *Player) VelX() float64 {
	return p.Body.GetLinearVelocity().X
}

func (p *Player) VelY() float64 {
	return p.Body.GetLinearVelocity().Y
}

func (p *Player) DirX() float64 {
	return math.Sin(p.Body.GetAngle())
}

func (p *Player) DirY() float64 {
	return math.Cos(p.Body.GetAngle())
}

func makePlayer(server *Server) *Player {
	bd := box2d.MakeB2BodyDef()	
	bd.Position.Set(0.0, 0.0)
	bd.Type = box2d.B2BodyType.B2_dynamicBody
	bd.FixedRotation = true
	bd.AllowSleep = true
	body := server.world.CreateBody(&bd)
	shape := box2d.MakeB2CircleShape()
	shape.M_radius = 0.5
	fd := box2d.MakeB2FixtureDef()
	fd.Shape = &shape
	fd.Density = 1.0
	fd.Restitution = 0.3
	body.CreateFixtureFromDef(&fd)
	test_renderable := &gp.Renderable{
		Id: 1,
		Color: 0,
		Size: 0.5, 
	}
	return &Player{
		id: atomic.AddUint32(&server.id_counter, 1),
		Input: make(chan *gp.Status),
		Body: body,
		renderables: []*gp.Renderable{test_renderable},
	}
}

type Client struct {
	server *Server

	// The websocket connection.
	conn *ws.Conn

	// Channel of outbound messages.
	send chan *ws.PreparedMessage

	player *Player
}

type Server struct {
	// Start of server time
	start time.Time
	last time.Time

	// Registered clients.
	clients map[*Client]bool

	// Outging updates from the server
	update chan []byte

	// Client join
	join chan *Client

	// Client leave
	leave chan *Client

	// Physics world
	world box2d.B2World

	// Entities
	entities []Entity
	entities_lock sync.RWMutex

	id_counter uint32
}

func makeServer() *Server {
	gravity := box2d.MakeB2Vec2(0.0, 0.0)

	// Construct a world object, which will hold and simulate the rigid bodies.
	world := box2d.MakeB2World(gravity)

	return &Server{
		start: time.Now(),
		last: time.Now(),
		clients: make(map[*Client]bool),
		update: make(chan []byte),
		join: make(chan *Client),
		leave: make(chan *Client),
		world: world,
		entities: make([]Entity, 0, 4),
		id_counter: 0,
	}
}

func (server *Server) run_events() {
	for {
		select {
		case client := <-server.join:
			server.clients[client] = true
			fmt.Println("Client joined: ", client.player.Id())
		case client := <-server.leave:
			if _, exists := server.clients[client]; exists {
				delete(server.clients, client)
				close(client.send)
				ded := client.player.Id()
				fmt.Println("Client left: ", ded)
				server.entities_lock.Lock()

				ded_index := 0
				for i, ent := range server.entities {
					if ent.Id() == ded {
						ded_index = i
						break
					}
				}
				if ded_index != 0 {
					server.entities = append(server.entities[:ded_index], server.entities[ded_index + 1:]...)
				}
				server.world.DestroyBody(client.player.Body)

				server.entities_lock.Unlock()
				fmt.Println("Unlock after player death")
			}
		case update := <-server.update:
			msg, err := ws.NewPreparedMessage(ws.BinaryMessage, update)
			if err != nil {
				log.Print("prepare_error:", err)
			}
			for client := range server.clients {
				select {
				case client.send <- msg:
				default:
					server.leave <- client
				}
			}
		}
	}
}

func (server *Server) run_simulation() {
	for {
		server.entities_lock.Lock()
		now := time.Now()
		delta := now.Sub(server.last).Seconds()
		server.last = now

		server_time := now.Sub(server.start).Seconds()
		server.world.Step(delta, 4, 4)
		update := &gp.Update{
			Time: server_time,
			Entity: make([]*gp.Entity, 0, len(server.entities)),
			CamX: 0.0,
			CamY: 0.0,
			CamScale: 1.5,
		}
		for _, ent := range server.entities {
			ent.Update(delta)

			// Create entity update packet
			ent_update := &gp.Entity{
				Id: ent.Id(),
				PosX: float32(ent.PosX()),
				PosY: float32(ent.PosY()),
				VelX: float32(ent.VelX()),
				VelY: float32(ent.VelY()),
				DirX: float32(ent.DirX()),
				DirY: float32(ent.DirY()),
				Renderable: ent.Renderables(),
			}
			fmt.Println(ent_update.PosX, ent_update.PosY)
			update.Entity = append(update.Entity, ent_update)
		}
		server.entities_lock.Unlock()
		out, err := proto.Marshal(update)
		if err != nil {
			log.Print(err)
		}
		packet := make([]byte, 2, len(out) + 2)
		binary.LittleEndian.PutUint16(packet, uint16(len(out)))
		packet = append(packet, out...)
		server.update <- packet
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
		c.server.leave <- c
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
		status := &gp.Status{}
		proto.Unmarshal(message, status)
		if c.player != nil {
			select {
				case c.player.Input <- status:
				default:
			}
		}
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

func handleClient(server *Server, w http.ResponseWriter, r *http.Request) {
	fmt.Println("Client connecting..")
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade error:", err)
		return
	}
	fmt.Println("A")

	client := &Client{
		server: server,
		conn: conn,
		send: make(chan *ws.PreparedMessage),
		player: makePlayer(server),
	}
	fmt.Println("B")

	server.entities_lock.Lock()
	fmt.Println("B2")
	server.entities = append(server.entities, client.player)
	server.entities_lock.Unlock()

	fmt.Println("C")
	server.join <- client
	go client.readUpdate()
	go client.sendUpdate()
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	fmt.Printf("Server listening on %s\n", *addr)
	server := makeServer()
	go server.run_events()
	go server.run_simulation()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleClient(server, w, r)
	})
	log.Fatal(http.ListenAndServe(*addr, nil))
}
