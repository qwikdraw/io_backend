package main

import (
	"flag"
	"log"
	"net/http"
	ws "github.com/gorilla/websocket"
	"fmt"
	gp "server/game_protocol"
	proto "github.com/golang/protobuf/proto"
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

	msg, err := ws.NewPreparedMessage(ws.BinaryMessage, []byte{3,2,1})
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
	update := &gp.Update{
		Time: 634.0,
		Remove: []uint32{1, 2, 3},
	}
	out, _ := proto.Marshal(update)
	fmt.Println(len(out), out)

	flag.Parse()
	log.SetFlags(0)
	fmt.Printf("Server listening on %s\n", *addr)
	http.HandleFunc("/", binary_response)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
