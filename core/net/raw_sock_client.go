package net

import (
	"io"
	"log"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/mdlayher/raw"
	"github.com/sirupsen/logrus"
)

type RawSockClient struct {
	//LocalSock
	conn     net.PacketConn
	connLock sync.RWMutex

	ifName       string
	ifi          *net.Interface
	sendChan     chan []byte
	recvChan     chan []byte
	firstConnect bool
	connected    bool
	doneChan     chan bool
}

func NewRawSockClient(ifName string, sendChan chan []byte, recvChan chan []byte) *RawSockClient {
	return &RawSockClient{
		ifName:       ifName,
		sendChan:     sendChan,
		recvChan:     recvChan,
		firstConnect: false,
		connected:    false,
		doneChan:     make(chan bool, 1),
	}
}

func (rsc *RawSockClient) Connect() error {
	logrus.Infof("Connecting to %s", rsc.ifName)
	var err error

	rsc.connLock.Lock()
	defer rsc.connLock.Unlock()

	rsc.ifi, err = net.InterfaceByName(rsc.ifName)
	if err != nil {
		return err
	}

	trialCount := 0
	for trialCount < MaximumConnectionTrialCount {
		rsc.conn, err = raw.ListenPacket(rsc.ifi, syscall.ETH_P_ALL, nil) // &raw.Config{LinuxSockDGRAM: true})
		if err != nil {
			time.Sleep(500 * time.Millisecond)
		} else {
			log.Println("Connected to ", rsc.conn.LocalAddr())
			rsc.firstConnect = true
			rsc.connected = true
			break
		}
		trialCount += 1
	}

	return err
}

func (rsc *RawSockClient) Close() error {
	return rsc.conn.Close()
}

func (rsc *RawSockClient) Start() {
	go rsc.reader()
	go rsc.writer()
	go rsc.reconnect()
	<-rsc.doneChan
}

func (rsc *RawSockClient) reconnect() {
	for {
		rsc.connLock.RLock()
		checkConnection := !rsc.connected
		rsc.connLock.RUnlock()

		if checkConnection {
			log.Println("Attempting to reconnect to data path: ", rsc.ifName)
			err := rsc.Connect()
			if err != nil {
				log.Println("Error re-connecting to data path: ", err)
			}
			time.Sleep(time.Second)
		}
	}
}

func (rsc *RawSockClient) reader() {
	var buf []byte
	if rsc.ifi != nil {
		buf = make([]byte, rsc.ifi.MTU)
	} else {
		buf = make([]byte, 1500)
	}

	for {
		rsc.connLock.RLock()
		connected := rsc.connected
		rsc.connLock.RUnlock()
		if connected {
			n, _, err := rsc.conn.ReadFrom(buf)
			if err != nil {
				log.Fatalf("failed to receive message: %v", err)
			}

			if err != nil {
				if err == io.EOF {
					rsc.connLock.Lock()
					rsc.connected = false
					err1 := rsc.conn.Close()
					if err1 != nil {
						log.Println(err1)
					}
					rsc.connLock.Unlock()
				}
			} else {
				if n >= 14 {
					//log.Println("Ethernet Type: ", buf[12], buf[13])
				}
				rsc.recvChan <- buf[0:n]
				if rsc.ifi != nil {
					buf = make([]byte, rsc.ifi.MTU)
				} else {
					buf = make([]byte, 1500)
				}
			}
		}
	}
}

func (rsc *RawSockClient) writer() {
	for {
		rsc.connLock.RLock()
		connected := rsc.connected
		rsc.connLock.RUnlock()

		select {

		case b := <-rsc.sendChan:
			if connected {
				n, err := rsc.conn.WriteTo(b, &raw.Addr{HardwareAddr: rsc.ifi.HardwareAddr})
				if err != nil {
					log.Println("Write Error")
					log.Println(err)
					log.Println(n)
					return
				} else {
					// log.Printf("%d bytes were sent", n)
				}
			}

		default:
			continue
		}
	}
}
