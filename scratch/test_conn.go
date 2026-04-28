package main

import (
	"fmt"
	"net"
	"time"
)

func main() {
	addr := "127.0.0.1:55000"
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		fmt.Printf("Connection failed: %v\n", err)
		return
	}
	conn.Close()
	fmt.Printf("Connection successful to %s\n", addr)
}
