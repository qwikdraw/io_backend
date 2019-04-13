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

type Player struct {
	world *World

	// The websocket connection.
	conn *ws.Conn

	// Buffered channel of outbound messages.
	send chan *ws.PreparedMessage
}

type World struct {
	// Start of server time
	start time.Time

	// Registered players.
	players map[*Player]bool

	// Input messages from the players.
	input chan []byte

	// Outging updates from the server
	update chan []byte

	// Player join
	join chan *Player

	// Player leave
	leave chan *Player
}

func makeWorld() *World {
	return &World{
		start: time.Now(),
		players: make(map[*Player]bool),
		input: make(chan []byte),
		update: make(chan []byte),
		join: make(chan *Player),
		leave: make(chan *Player),
	}
}

func (world *World) run_networking() {
	for {
		select {
		case player := <-world.join:
			world.players[player] = true
		case player := <-world.leave:
			if _, exists := world.players[player]; exists {
				delete(world.players, player)
				close(player.send)
				player.conn.Close()
			}
		case message := <-world.input:
			// Handle player input here.
			log.Print("player: ", message)
		case update := <-world.update:
			msg, err := ws.NewPreparedMessage(ws.BinaryMessage, update)
			if err != nil {
				log.Print("prepare_error:", err)
			}
			for player := range world.players {
				select {
				case player.send <- msg:
				default:
					close(player.send)
					delete(world.players, player)
				}
			}
		}
	}
}

func (world *World) run_simulation() {
	for {
		now := time.Now()
		server_time := now.Sub(world.start).Seconds()

		test_renderable := &gp.Renderable{
			Id: 1,
			Color: 0,
			Size: 1.0, 
		}
		test_entity := &gp.Entity{
			Id: 23,
			PosX: float32(math.Sin(server_time)),
			PosY: float32(math.Cos(server_time)),
			Renderable: []*gp.Renderable{test_renderable},
		}
		update := &gp.Update{
			Time: server_time,
			Entity: []*gp.Entity{test_entity},
			Remove: []uint32{1, 2},
		}
		out, _ := proto.Marshal(update)
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

func (p *Player) sendUpdate() {
	defer func() {
		p.conn.Close()
		p.world.leave <- p
	}()

	for {
		select {
		case msg, ok := <-p.send:
			
			if !ok {
				// The hub closed the channel.
				p.conn.WriteMessage(ws.CloseMessage, []byte{})
				return
			}

			err := p.conn.WritePreparedMessage(msg)
			if err != nil {
				log.Print("error sending update: ", p)
			}
		}
	}
}

func handlePlayer(world *World, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade error:", err)
		return
	}

	player := &Player{
		world: world,
		conn: conn,
		send: make(chan *ws.PreparedMessage),
	}
	world.join <- player

	//go player.readInput()
	go player.sendUpdate()
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	fmt.Printf("Server listening on %s\n", *addr)
	world := makeWorld()
	go world.run_networking()
	go world.run_simulation()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handlePlayer(world, w, r)
	})
	log.Fatal(http.ListenAndServe(*addr, nil))
}
