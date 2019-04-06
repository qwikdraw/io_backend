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
)

var addr = flag.String("addr", "0.0.0.0:3000", "http service address")

var upgrader = ws.Upgrader{
	CheckOrigin: func(r *http.Request) bool {return true},
	Subprotocols: []string{"binary"},
}

func binary_response(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	test_entity := &gp.Entity{
		Id: 23,
		Renderable: []uint32{42, 56},
	}
	update := &gp.Update{
		Time: 634.0,
		Update: []*gp.Entity{test_entity},
		Remove: []uint32{1, 2, 3},
	}
	out, _ := proto.Marshal(update)
	packet := make([]byte, 2, len(out) + 2)
	binary.LittleEndian.PutUint16(packet, uint16(len(out)))
	packet = append(packet, out...)

	fmt.Println(packet)

	msg, err := ws.NewPreparedMessage(ws.BinaryMessage, packet)
	if err != nil {
		log.Print("prepare:", err)
	}

	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		err = c.WritePreparedMessage(msg)
		if err != nil {
			log.Print("error sending test message:", err)
		}
	}
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	fmt.Printf("Server listening on %s\n", *addr)
	http.HandleFunc("/", binary_response)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
