package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

type reply struct {
	data []byte
	addr *net.UDPAddr
}

var (
	server  *net.UDPAddr
	replyCh chan *reply
	count   int32
)

func main() {
	addr, err := net.ResolveUDPAddr("udp", "0.0.0.0:53")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't resolve service address: %v\n", err)
		os.Exit(1)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't listen on udp port: %v\n", err)
		os.Exit(1)
	}
	replyCh = make(chan *reply)
	go sendReply(conn)
	server, err = net.ResolveUDPAddr("udp", "8.8.4.4:53")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't resolve server address: %v\n", err)
		os.Exit(1)
	}
	for {
		reqData := make([]byte, 2048)
		nr, client, err := conn.ReadFromUDP(reqData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read from client: %v\n", err)
			//checkTempErr(err)
			continue
		}
		go proxyClientDNS(reqData[:nr], client)
	}
}

func checkTempErr(err error) {
	if err2, ok := err.(*net.OpError); ok && !err2.Temporary() {
		fmt.Fprintf(os.Stderr, "FATAL network error\n")
		os.Exit(1)
	}
}

func proxyClientDNS(reqData []byte, client *net.UDPAddr) {
	c := atomic.AddInt32(&count, 1)
	domain := extractDomain(reqData)
	fmt.Printf("(%3d) Receive DNS request from %24v: %v\n", c, client, domain)

	conn, err := net.DialUDP("udp", nil, server)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't dial server: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	_, err = conn.Write(reqData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write to server: %v\n", err)
		return
	}
	t := time.Now().Add(time.Duration(5) * time.Second)
	conn.SetReadDeadline(t)
	var rep []byte
	for {
		repData := make([]byte, 2048)
		nr, err := conn.Read(repData)
		if err != nil {
			err2, ok := err.(*net.OpError)
			if ok && err2.Timeout() {
				break
			}
			fmt.Fprintf(os.Stderr, "Failed to read from server: %v\n", err)
			break
		}
		rep = repData[:nr]
	}
	replyCh <- &reply{rep, client}

	atomic.AddInt32(&count, -1)
}

func extractDomain(reqData []byte) string {
	var nodes []string
	i := byte(12)
	for {
		l := reqData[i]
		if l == 0 {
			break
		}
		nodes = append(nodes, string(reqData[i+1:i+l+1]))
		i += l + 1
	}
	return strings.Join(nodes, ".")
}

func sendReply(conn *net.UDPConn) {
	for {
		aReply := <-replyCh
		_, err := conn.WriteToUDP(aReply.data, aReply.addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to write to client: %v\n", err)
			checkTempErr(err)
		}
	}
}
