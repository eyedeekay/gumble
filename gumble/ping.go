package gumble

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/eyedeekay/sam3/helper"
)

// PingResponse contains information about a server that responded to a UDP
// ping packet.
type PingResponse struct {
	// The address of the pinged server.
	Address net.Addr
	// The round-trip time from the client to the server.
	Ping time.Duration
	// The server's version. Only the Version field and SemanticVersion method of
	// the value will be valid.
	Version Version
	// The number users currently connected to the server.
	ConnectedUsers int
	// The maximum number of users that can connect to the server.
	MaximumUsers int
	// The maximum audio bitrate per user for the server.
	MaximumBitrate int
}

// I2PPing is like Ping, but for a Mumble server hosted on I2P
func I2PPing(address string, interval, timeout time.Duration) (*PingResponse, error) {
	if timeout < 0 {
		return nil, errors.New("gumble: timeout must be positive")
	}
	deadline := time.Now().Add(timeout)
	//	conn, err := sam.I2PDatagramSession("mumvle-ping", "127.0.0.1:7656", "mumble-ping")
	//	conn, err := sam.I2PDatagramSession("mumble-ping", "127.0.0.1:7656", "mumble-ping")
	sconn, err := sam.I2PStreamSession("mumble-ping", "127.0.0.1:7656", "mumble-ping")
	if err != nil {
		return nil, err
	}
	defer sconn.Close()
	log.Println("mumble-ping")
	netaddr, err := sconn.Lookup(strings.Split(address, ":")[0])
	if err != nil {
		return nil, err
	}
	conn, err := sconn.DialI2P(netaddr)
	if err != nil {
		return nil, err
	}
	conn.SetReadDeadline(deadline)
	return PingConn(conn, interval)
}

//
func PingConn(conn net.Conn, interval time.Duration) (*PingResponse, error) {
	var (
		idsLock sync.Mutex
		ids     = make(map[string]time.Time)
	)

	buildSendPacket := func() {
		var packet [12]byte
		if _, err := rand.Read(packet[4:]); err != nil {
			return
		}
		id := string(packet[4:])
		idsLock.Lock()
		ids[id] = time.Now()
		idsLock.Unlock()
		conn.Write(packet[:])
	}

	if interval > 0 {
		end := make(chan struct{})
		defer close(end)
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					buildSendPacket()
				case <-end:
					return
				}
			}
		}()
	}

	buildSendPacket()

	for {
		var incoming [24]byte
		if _, err := io.ReadFull(conn, incoming[:]); err != nil {
			return nil, err
		}
		id := string(incoming[4:12])
		idsLock.Lock()
		sendTime, ok := ids[id]
		idsLock.Unlock()
		if !ok {
			continue
		}

		return &PingResponse{
			Address: conn.RemoteAddr().(*net.UDPAddr),
			Ping:    time.Since(sendTime),
			Version: Version{
				Version: binary.BigEndian.Uint32(incoming[0:]),
			},
			ConnectedUsers: int(binary.BigEndian.Uint32(incoming[12:])),
			MaximumUsers:   int(binary.BigEndian.Uint32(incoming[16:])),
			MaximumBitrate: int(binary.BigEndian.Uint32(incoming[20:])),
		}, nil
	}
}

// Ping sends a UDP ping packet to the given server. If interval is positive,
// the packet is retransmitted at every interval.
//
// Returns a PingResponse and nil on success. The function will return nil and
// an error if a valid response is not received after the given timeout.
func Ping(address string, interval, timeout time.Duration) (*PingResponse, error) {
	if timeout < 0 {
		return nil, errors.New("gumble: timeout must be positive")
	}
	deadline := time.Now().Add(timeout)
	conn, err := net.DialTimeout("udp", address, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetReadDeadline(deadline)

	return PingConn(conn, interval)
}
