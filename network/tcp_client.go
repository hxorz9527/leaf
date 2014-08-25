package network

import (
	"github.com/name5566/leaf/log"
	"net"
	"sync"
	"time"
)

type TCPClient struct {
	sync.Mutex
	Addr            string
	ConnNum         int
	ConnectInterval time.Duration
	PendingWriteNum int
	NewAgent        func(*TCPConn) Agent
	conns           ConnSet
	wg              sync.WaitGroup
	closeFlag       bool
	disp            Dispatcher
}

func (client *TCPClient) Start() {
	client.init()

	for i := 0; i < client.ConnNum; i++ {
		go client.connect()
	}
}

func (client *TCPClient) init() {
	client.Lock()
	defer client.Unlock()

	if client.ConnNum <= 0 {
		client.ConnNum = 1
		log.Release("invalid ConnNum, reset to %v", client.ConnNum)
	}
	if client.ConnectInterval <= 0 {
		client.ConnectInterval = 3 * time.Second
		log.Release("invalid ConnectInterval, reset to %v", client.ConnectInterval)
	}
	if client.PendingWriteNum <= 0 {
		client.PendingWriteNum = 100
		log.Release("invalid PendingWriteNum, reset to %v", client.PendingWriteNum)
	}
	if client.NewAgent == nil {
		log.Fatal("NewAgent must not be nil")
	}
	if client.conns != nil {
		log.Fatal("client is running")
	}

	client.conns = make(ConnSet)
	client.closeFlag = false
}

func (client *TCPClient) dial() net.Conn {
	for {
		conn, err := net.Dial("tcp", client.Addr)
		if err == nil || client.closeFlag {
			return conn
		}

		log.Release("connect to %v error: %v", client.Addr, err)
		time.Sleep(client.ConnectInterval)
		continue
	}
}

func (client *TCPClient) connect() {
	conn := client.dial()
	if conn == nil {
		return
	}

	client.Lock()
	if client.closeFlag {
		conn.Close()
		client.Unlock()
		return
	}
	client.conns[conn] = struct{}{}
	client.Unlock()

	client.wg.Add(1)

	tcpConn := NewTCPConn(conn, client.PendingWriteNum)
	agent := client.NewAgent(tcpConn)
	client.handle(agent)

	// cleanup
	tcpConn.Close()
	client.Lock()
	if client.conns != nil {
		delete(client.conns, conn)
	}
	client.Unlock()

	client.wg.Done()
}

func (client *TCPClient) handle(agent Agent) {
	for {
		id, msg, err := agent.Read()
		if err != nil {
			break
		}

		handler := client.disp.Handler(id)
		if handler == nil {
			break
		}
		handler(agent, msg)
	}

	agent.OnClose()
}

func (client *TCPClient) Close() {
	client.Lock()
	client.closeFlag = true
	if client.conns != nil {
		for conn, _ := range client.conns {
			conn.Close()
		}
		client.conns = nil
	}
	client.Unlock()

	client.wg.Wait()
}

func (client *TCPClient) RegHandler(id interface{}, handler Handler) {
	client.disp.RegHandler(id, handler)
}