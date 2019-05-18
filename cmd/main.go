package main

import (
	"time"
	"fmt"
	"github.com/gyf304/svpn/mqttconn"
)

func main() {
	uri := "mqtt://broker.hivemq.com/test"
	conn, err := mqttconn.CreateMQTTConnFromURL(uri)
	if err != nil {
		panic(err)
	}
	buffer := make([]byte, 65536)
	for true {
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println(err)
			continue
		}
		str := string(buffer[0:n])
		fmt.Println(str)
	}
}
