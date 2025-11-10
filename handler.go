package main

import (
	"fmt"
	"io"
	"log"
	"net"
)

// ConnectionHandler handles the port forwarding logic
type ConnectionHandler struct {
	sshPool *SSHClientPool
}

// NewConnectionHandler creates a new connection handler
func NewConnectionHandler(sshPool *SSHClientPool) *ConnectionHandler {
	return &ConnectionHandler{
		sshPool: sshPool,
	}
}

// HandleConnection handles a single forwarded connection
func (ch *ConnectionHandler) HandleConnection(localConn net.Conn, cfg ForwardConfig) {
	defer localConn.Close()

	sshClient, err := ch.sshPool.GetClient(cfg.JumpHost, cfg.JumpPort, cfg.KeyPath)
	if err != nil {
		log.Printf("Error: Failed to get SSH client: %v", err)
		return
	}

	remoteAddr := fmt.Sprintf("%s:%d", cfg.RemoteHost, cfg.Port)
	remoteConn, err := sshClient.Dial("tcp", remoteAddr)
	if err != nil {
		// Connection might be stale (server timeout), try reconnecting once
		log.Printf("Connection failed, attempting reconnect: %v", err)
		ch.sshPool.RemoveClient(cfg.JumpHost, cfg.JumpPort)

		sshClient, err = ch.sshPool.GetClient(cfg.JumpHost, cfg.JumpPort, cfg.KeyPath)
		if err != nil {
			log.Printf("Error: Failed to reconnect: %v", err)
			return
		}

		remoteConn, err = sshClient.Dial("tcp", remoteAddr)
		if err != nil {
			log.Printf("Error: Failed to dial %s after reconnect: %v", remoteAddr, err)
			return
		}
	}
	defer remoteConn.Close()

	log.Printf("Forwarding: :%d -> %s", cfg.Port, remoteAddr)

	done := make(chan struct{}, 2)

	go func() {
		io.Copy(remoteConn, localConn)
		done <- struct{}{}
	}()

	go func() {
		io.Copy(localConn, remoteConn)
		done <- struct{}{}
	}()

	<-done
	log.Printf("Connection closed: :%d", cfg.Port)
}

// ListenOnPort listens on a specific port and forwards connections
func (ch *ConnectionHandler) ListenOnPort(cfg ForwardConfig) {
	listenAddr := fmt.Sprintf("%s:%d", cfg.LocalIP, cfg.Port)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return
	}
	defer listener.Close()

	log.Printf("Listening on %s", listenAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Error: Accept error on %s: %v", listenAddr, err)
			return
		}

		go ch.HandleConnection(conn, cfg)
	}
}
