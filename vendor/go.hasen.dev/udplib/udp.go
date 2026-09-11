package udplib

import (
	"net"
	"time"
)

type UDPHandler[T any] func(data *T, message []byte) []byte

// Listen binds 127.0.0.1:port. Panics on bind error. The port is ready when
// this returns.
func Listen(port int) *net.UDPConn {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: port,
	})
	if err != nil {
		panic(err)
	}
	return conn
}

// StartUDPServer listens on 127.0.0.1:port and calls handler for each
// loopback datagram. The handler return value is sent back to the sender.
// Non-loopback senders are dropped. done, if non-nil, stops the loop when
// set true (the caller sets it; this function then closes the socket).
func StartUDPServer[T any](s *T, port int, handler UDPHandler[T], done *bool) {
	Serve(Listen(port), s, handler, done)
}

// Serve reads from an already-bound loopback socket. See StartUDPServer.
func Serve[T any](conn *net.UDPConn, s *T, handler UDPHandler[T], done *bool) {
	var fallbackDone bool
	if done == nil {
		done = &fallbackDone
	}

	var inputBuffer = make([]byte, 1024)
	for !(*done) {
		n, sender, err := conn.ReadFromUDP(inputBuffer)
		if err != nil {
			continue
		}
		if !sender.IP.IsLoopback() {
			continue
		}
		resp := handler(s, inputBuffer[:n])
		_, err = conn.WriteToUDP(resp, sender)
		if err != nil {
			continue
		}
	}
	conn.Close()
}

const defaultSendWait = 100 * time.Millisecond

// SendUDPMessage sends cmd to 127.0.0.1:port and waits up to 100ms for a reply.
// responseBuffer is optional; a 1KB buffer is used if it is nil.
func SendUDPMessage(port int, cmd []byte, responseBuffer []byte) (resp []byte, ok bool) {
	return SendUDP(port, cmd, responseBuffer, defaultSendWait)
}

func SendUDPMessageString(port int, msg string) (resp string, ok bool) {
	b, ok := SendUDPMessage(port, []byte(msg), nil)
	return string(b), ok
}

// SendUDP is SendUDPMessage with an explicit read deadline. wait <= 0 uses
// the 100ms default.
func SendUDP(port int, cmd []byte, responseBuffer []byte, wait time.Duration) (resp []byte, ok bool) {
	if wait <= 0 {
		wait = defaultSendWait
	}
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: port,
	})
	if err != nil {
		return
	}
	defer conn.Close()
	if _, err = conn.Write(cmd); err != nil {
		return
	}
	if err = conn.SetReadDeadline(time.Now().Add(wait)); err != nil {
		return
	}
	if responseBuffer == nil {
		responseBuffer = make([]byte, 1024)
	}
	n, err := conn.Read(responseBuffer)
	if n > 0 {
		resp = responseBuffer[:n]
	}
	if err != nil {
		return
	}
	ok = true
	return
}

// FreePort returns an unused loopback UDP port. The port is released before
// return; a later bind can still fail if something else takes it.
func FreePort() (int, error) {
	ln, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		return 0, err
	}
	port := ln.LocalAddr().(*net.UDPAddr).Port
	if err := ln.Close(); err != nil {
		return 0, err
	}
	return port, nil
}
