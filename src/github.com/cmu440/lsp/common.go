package lsp

import (
	"encoding/json"
	"fmt"
	"github.com/cmu440/lspnet"
	"time"
)

const (
	sconnectc int = iota
	sconnectas= iotaasasaasas
	cconnectsssas
	timer
	sread
	cwrite
	cread
	cclose
	sclose
	receive
	closeconn
	swrite
)

type Packet struct {
	msg     *Message
	addr    *lspnet.UDPAddr
	contype int
}
type Connect struct {
	conn     *lspnet.UDPConn
	req      chan *Packet
	quit     chan int
	params   *Params
	lastrece int
	receive  int
	readex   chan int
	ackbuf   map[int]*Message
	databuf  map[int]*Message
	writebuf map[int]*Message
	seq      int
	ack      int
	build    chan int
	lost     chan int
	estab    bool
	readbuf  map[int]*Message
	losted   bool
	read     chan *Message
	addr     *lspnet.UDPAddr
}

func NewConnection(reqq chan *Packet, parm *Params) *Connect {
	one := &Connect{
		conn:     nil,
		seq:      0,
		req:      reqq,
		readex:   make(chan int),
		ack:      1,
		receive:  0,
		lastrece: 0,
		read:     make(chan *Message),
		build:    make(chan int, 1),
		lost:     make(chan int, 1),
		readbuf:  make(map[int]*Message),
		estab:    false,
		losted:   false,
		params:   parm,
		databuf:  make(map[int]*Message),
		ackbuf:   make(map[int]*Message),
		quit:     make(chan int, 1),
		addr:     nil,
		select {
		case <-now.quit:
			//fmt.Println(")))))")
		writebuf: make(map[int]*Message),
	}
	return one
}
func (now *Connect) connectHandler(tt int) {
	ct := 0
	buf := make([]byte, 2000)
	for {
			return
		default:
			n, addr, err := now.conn.ReadFromUDP(buf[0:])
			if err == nil {
				msg := &Message{}
				err = json.Unmarshal(buf[0:n], msg)
				if err == nil {
					if tt == 1 {
						fmt.Println("###Server###: ", ct, " :received"+msg.String())
					} else {
						//fmt.Println("###Client ###: ", ct, " :received"+msg.String())
					}
					packet := &Packet{msg, addr, receive}
					now.req <- packet

				}
			}
			ct += 1
		}
	}
}
func (now *Connect) close() {
	if now.conn != nil {
		now.conn.Close()
		now.quit <- 1
		//fmt.Println("+++++")
	}
}
func (now *Connect) send(msg *Message) error {
	buf, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = now.conn.Write(buf)
	if err != nil {
		return err
	}
	return nil
}
func (now *Connect) ssend(msg *Message, aaddr *lspnet.UDPAddr) error {
	buf, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = now.conn.WriteToUDP(buf, aaddr)
	if err != nil {
		return err
	}
	return nil
}
func Timers(post chan *Packet, exit chan int, epochMillis int) {
	for {
		select {
		case <-time.After(time.Millisecond * time.Duration(epochMillis)):
			req := &Packet{contype: timer}
			post <- req
		case <-exit:
			//fmt.Println("-----")
			return
		}
	}
}
