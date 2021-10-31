package main

import (
	"fmt"
	"net"
	"strings"
)

func main() {
	udpAddr, _ := net.ResolveUDPAddr("udp4", "127.0.0.1:12345")

	//监听端口
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println(err)
	}
	defer udpConn.Close()

	fmt.Println("udp listening ... ")

	//udp不需要Accept
	for {
		handleConnection(udpConn)
	}
}

// 读取消息
func handleConnection(udpConn *net.UDPConn) {

	// 读取数据
	buf := make([]byte, 1024)
	len, udpAddr, err := udpConn.ReadFromUDP(buf)
	if err != nil {
		return
	}
	logContent := strings.Replace(string(buf), "\n", "", 1)
	fmt.Println("server read len:", len)
	fmt.Println("server read data:", logContent)
	fmt.Println("client ip:", udpAddr)
}
