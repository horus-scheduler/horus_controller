package net

import (
	"io"
	"log"
	"net"
	"sync"
	"time"
)

type UnixSockClient struct {
	//LocalSock
	conn     net.Conn
	connLock sync.RWMutex

	sockName     string
	sendChan     chan []byte
	recvChan     chan []byte
	firstConnect bool
	connected    bool
	doneChan     chan bool
}

func NewUnixSockClient(sockName string, sendChan chan []byte, recvChan chan []byte) *UnixSockClient {
	return &UnixSockClient{
		sockName:     sockName,
		sendChan:     sendChan,
		recvChan:     recvChan,
		firstConnect: false,
		connected:    false,
		doneChan:     make(chan bool, 1),
	}
}

func (usc *UnixSockClient) Connect() error {
	usc.connLock.Lock()
	defer usc.connLock.Unlock()

	var err error
	trialCount := 0
	for trialCount < MaximumConnectionTrialCount {
		usc.conn, err = net.Dial("unixpacket", usc.sockName)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
		} else {
			log.Println("Connected to ", usc.conn.RemoteAddr())
			usc.firstConnect = true
			usc.connected = true
			break
		}
		trialCount += 1
	}

	return err
}

func (usc *UnixSockClient) Close() error {
	return usc.conn.Close()
}

func (usc *UnixSockClient) Start() {
	go usc.reader()
	go usc.writer()
	go usc.reconnect()
	<-usc.doneChan
}

func (usc *UnixSockClient) reconnect() {
	for {
		usc.connLock.RLock()
		checkConnection := !usc.connected
		usc.connLock.RUnlock()

		if checkConnection {
			log.Println("Attempting to reconnect to data path: ", usc.sockName)
			err := usc.Connect()
			if err != nil {
				log.Println("Error re-connecting to data path: ", err)
			}
			time.Sleep(time.Second)
		}
	}
}

func (usc *UnixSockClient) reader() {
	buf := make([]byte, 1024)
	for {
		usc.connLock.RLock()
		connected := usc.connected
		usc.connLock.RUnlock()

		if connected {
			n, err := usc.conn.Read(buf[:])
			if err != nil {
				if err == io.EOF {
					usc.connLock.Lock()
					usc.connected = false
					err1 := usc.conn.Close()
					if err1 != nil {
						log.Println(err1)
					}
					usc.connLock.Unlock()
				}
			} else {
				usc.recvChan <- buf[0:n]
				buf = make([]byte, 1024)
			}
		}
	}
}

func (usc *UnixSockClient) writer() {
	for {
		usc.connLock.RLock()
		connected := usc.connected
		usc.connLock.RUnlock()
		select {
		case b := <-usc.sendChan:
			if connected {
				_, err := usc.conn.Write(b)
				if err != nil {
					//log.Println("Write error:", err)
				}
			}
		default:
			continue
		}
	}
}
