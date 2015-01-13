// Contains the implementation of a LSP server.

package lsp

import (
	"errors"
	"fmt"
	"github.com/cmu440/lspnet"
	"strconv"
)

type fk struct {
	databuf map[int]*Message
	lastEpo int
	rece    int
	addr    *lspnet.UDPAddr
	params  *Params
}
type server struct {
	listenconn *Connect
	initparams *Params
	req        chan *Packet
	over       chan *Packet
	reply      chan *Packet
	closeans   chan *Packet
	writeans   chan *Packet
	conn       map[int]*Connect
	read       chan *Message
	readlost   chan int
	readclose  bool
	connCt     int
	banclose   map[int]*fk
	exit       chan int
	closepend  map[int]bool
	closeack   map[int]bool
	sendct     map[int]int
	addmap     map[string]int
	exitEpo    chan int
	lostall    chan *Message
	// id->1
	pendconn map[int]int
	readnum  int
	estab    bool
	readct   map[int]int
	readall  chan *Message
}

// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	s := &server{
		initparams: params,
		req:        make(chan *Packet, 1000),
		reply:      make(chan *Packet, 10000),
		writeans:   make(chan *Packet, 100),
		closeans:   make(chan *Packet, 100),
		sendct:     make(map[int]int),
		readall:    make(chan *Message, 1),
		conn:       make(map[int]*Connect),
		over:       make(chan *Packet, 100),
		closepend:  make(map[int]bool),
		pendconn:   make(map[int]int),
		closeack:   make(map[int]bool),
		addmap:     make(map[string]int),
		readlost:   make(chan int, 1),
		banclose:   make(map[int]*fk),
		readclose:  false,
		lostall:    make(chan *Message),
		exit:       make(chan int, 1),
		exitEpo:    make(chan int, 1),
		readct:     make(map[int]int),
		estab:      false,
		readnum:    0,
		connCt:     1,
	}
	fmt.Println("conf: ", s.initparams.EpochLimit, s.initparams.EpochMillis)
	s.listenconn = NewConnection(s.req, s.initparams)
	err := s.listenconn.listen(":" + strconv.Itoa(port))
	if err != nil {
		return nil, err
	}
	//fmt.Println("------")
	go s.handler()
	go s.listenconn.connectHandler(1)
	go Timers(s.req, s.exitEpo, params.EpochMillis)
	fmt.Println("server up")
	return s, nil
}
func (now *Connect) listen(hostport string) error {
	serverAddr, err := lspnet.ResolveUDPAddr("udp", hostport)
	if err != nil {
		return nil
	}
	conn, err := lspnet.ListenUDP("udp", serverAddr)
	if err != nil {
		return err
	}
	now.conn = conn
	now.addr = serverAddr
	return nil
}
func (s *server) handleEpoch() {
	for k, v := range s.banclose {
		v.rece += 1
		if v.rece-v.lastEpo >= v.params.EpochLimit {
			delete(s.banclose, k)
		} else {
			for _, vv := range v.databuf {
				s.listenconn.ssend(vv, v.addr)
			}
		}
	}
	for i, v := range s.conn {
		v.receive += 1
		k, ok := s.pendconn[i]
		fmt.Println("#time#: client ", i, v.receive, v.lastrece, v.params.EpochLimit)
		if v.receive-v.lastrece+1 > v.params.EpochLimit {
			if ok {
				fmt.Println("lost client ", i, "before establish")
				if s.readclose {
					s.closeack[i] = true
					packet := &Packet{nil, nil, sclose}
					s.req <- packet
				} else {
					delete(s.addmap, v.addr.String())
					delete(s.readct, i)
					delete(s.closeack, i)
					delete(s.closepend, i)
					delete(s.pendconn, i)
					delete(s.conn, i)
					delete(s.sendct, i)
				}
			} else {
				fmt.Println("lost client ", i, "in the middle")
				s.conn[i].losted = true
				msg := &Message{ConnID: i}
				packet := &Packet{msg, nil, -1}
				s.reply <- packet

				if s.closepend[i] == true {
					//fmt.Println("????")
					s.closeack[i] = true
					packet := &Packet{nil, nil, sclose}
					s.req <- packet
				} else {
					//fmt.Println("000000000000")
					//s.closepend[i] = true
					msg := &Message{ConnID: i}
					packet := &Packet{msg, v.addr, sclose}
					fmt.Println("lost to handle close: ", i, " with ", len(s.conn[i].databuf), len(s.conn[i].writebuf))
					s.handleclosecon(packet)
				}
			}
		} else {
			if ok {
				msg := NewAck(k, 0)
				s.listenconn.ssend(msg, v.addr)
				for _, temp := range v.databuf {
					fmt.Println("resend ", temp.String())
					s.listenconn.ssend(temp, v.addr)
				}
			} else {
				for _, temp := range v.ackbuf {
					//fmt.Println("#Server# rsend ack ", temp.SeqNum, " to client: ", i)
					s.listenconn.ssend(temp, v.addr)
				}
				//resend those data buf in case that the server did not send acked
				for _, temp := range v.databuf {
					fmt.Println("#Server# rsend package ", temp.SeqNum, " to client: ", i)
					s.listenconn.ssend(temp, v.addr)
				}
			}
		}
	}
}
func (s *server) handleReceive(pak *Packet) {
	if s.closepend[pak.msg.ConnID] == true && pak.msg.Type != MsgAck {
		return
	}
	if s.closepend[pak.msg.ConnID] == true && len(s.conn[pak.msg.ConnID].writebuf) == 0 && len(s.conn[pak.msg.ConnID].databuf) == 0 {
		s.closeack[pak.msg.ConnID] = true
		packet := &Packet{nil, nil, sclose}
		s.req <- packet
		return
	}
	//fmt.Println("#server# receive", pak.msg.String())
	switch pak.msg.Type {
	case MsgConnect:
		hold := pak.addr.String()
		_, ok := s.addmap[hold]
		//fmt.Println("----", ok)
		if !ok {
			s.addmap[hold] = s.connCt
			cc := NewConnection(nil, s.initparams)
			cc.addr = pak.addr
			cc.seq = 1
			msg := NewAck(s.connCt, 0)

			s.listenconn.ssend(msg, cc.addr)
			s.conn[s.connCt] = cc
			//fmt.Println("+++", ok)
			s.closepend[s.connCt] = false
			s.closeack[s.connCt] = false
			s.readct[s.connCt] = 1
			s.pendconn[s.connCt] = 1
			s.sendct[s.connCt] = 1
			//fmt.Println("#Server#  new connection, give connID:", s.connCt)
			s.connCt += 1
		} else {
			idd := s.addmap[hold]
			_, ok2 := s.pendconn[idd]
			v, ok := s.conn[idd]
			if ok2 == false && ok == true {
				fmt.Println("#server# receive not closed singnal from client ", idd)
				v.lastrece = v.receive
			}
			if ok2 == true && ok == true {
				msg := NewAck(idd, 0)
				s.listenconn.ssend(msg, s.conn[idd].addr)
			}
		}
	case MsgData:
		_, ok2 := s.pendconn[pak.msg.ConnID]
		v, ok := s.conn[pak.msg.ConnID]
		if pak.msg.SeqNum > 0 && ok2 == true {
			delete(s.pendconn, pak.msg.ConnID)
			v.estab = true
			v.losted = false
			ok2 = false
			//	fmt.Println("#Server# confirmed establish from client: ", pak.msg.ConnID)
		}
		//fmt.Println("#server# ", pak.msg.SeqNum, " now ack: ", v.ack)
		if ok == true && ok2 == false && pak.msg.SeqNum >= v.ack {
			//if pak.msg.ConnID == 1 {
			////	fmt.Println("123123")
			//}
			v.lastrece = v.receive

			ackk := NewAck(pak.msg.ConnID, pak.msg.SeqNum)
			s.listenconn.ssend(ackk, v.addr)
			//fmt.Println("send ack to acknowledge: ", pak.msg.SeqNum, " for client: ", pak.msg.ConnID)
			_, ex := v.ackbuf[pak.msg.SeqNum]
			v.ackbuf[pak.msg.SeqNum] = ackk
			//fmt.Println("receive", pak.msg.String(), pak.msg.SeqNum, v.ack)
			if pak.msg.SeqNum-v.params.WindowSize >= v.ack {
				for i := v.ack; i <= pak.msg.SeqNum-v.params.WindowSize; i++ {
					delete(v.ackbuf, i)
				}
				v.ack = pak.msg.SeqNum - v.params.WindowSize + 1
			}
			//fmt.Println("now ack has changed to: ", v.ack)
			if !ex {
				v.readbuf[pak.msg.SeqNum] = pak.msg
				packet := &Packet{nil, nil, sread}
				s.req <- packet
				/*if pak.msg.SeqNum == s.readct[pak.msg.ConnID] {
				v.read <- pak.msg
				delete(v.readbuf, s.readct[pak.msg.ConnID])

				s.readct[pak.msg.ConnID] += 1
				for i := s.readct[pak.msg.ConnID]; i < s.readct[pak.msg.ConnID]+v.params.WindowSize; i++ {
					if pp, okk := v.readbuf[i]; okk == true {
						v.read <- pp
						delete(v.readbuf, i)
					} else {
						break
					}
				}
				*/
			}
		}
	case MsgAck:
		//idd := s.addmap[hold]
		_, ok2 := s.pendconn[pak.msg.ConnID]
		v, ok := s.conn[pak.msg.ConnID]
		//fmt.Println("#server# receive ack")
		if ok2 == false && ok == true && pak.msg.SeqNum == 0 {
			fmt.Println("#server# receive not closed singnal from client ", pak.msg.ConnID)
			hold := NewAck(pak.msg.ConnID, 0)
			s.listenconn.ssend(hold, v.addr)
			v.lastrece = v.receive
			return
		}
		nani, check := s.banclose[pak.msg.ConnID]
		if check == true {
			nani.lastEpo = nani.rece
			delete(nani.databuf, pak.msg.SeqNum)
			if len(nani.databuf) == 0 {
				delete(s.banclose, pak.msg.ConnID)
			}
			return
		}
		if pak.msg.SeqNum > 0 && ok2 == true {
			delete(s.pendconn, pak.msg.ConnID)
			v.estab = true
			v.losted = false
			ok2 = false
			//fmt.Println("#Server# confirmed establish from client: ", pak.msg.ConnID)
		}
		if ok == true && ok2 == false && pak.msg.SeqNum == 0 {
			v.lastrece = v.receive
			return
		}
		if ok == true && ok2 == false && pak.msg.SeqNum >= v.seq && pak.msg.SeqNum <= v.seq+v.params.WindowSize {
			v.lastrece = v.receive
			//fmt.Println("receive ack for packet ", pak.msg.SeqNum, "for client ", pak.msg.ConnID, "\t now ", v.seq, v.ack, len(v.databuf)-1)
			delete(v.databuf, pak.msg.SeqNum)
			if pak.msg.SeqNum == v.seq {
				hold := v.seq
				v.seq += 1

				ct := 1
				//find the maximum window sliding
				for i := v.seq; len(v.databuf) != 0 && i < hold+v.params.WindowSize-1; i++ {
					_, ok := v.databuf[i]
					if ok {
						break
					} else {
						v.seq += 1
						ct += 1
					}
				}
				// because the sliding try to send the maximum new data
				//fmt.Println(hold, "----", len(v.writebuf))
				///for i, kk := range v.writebuf {
				//	fmt.Println("#write#: ", i, "\t", kk.String())
				//}
				for i := hold + v.params.WindowSize; len(v.writebuf) != 0 && i < hold+v.params.WindowSize+ct; i++ {
					vv, okk := v.writebuf[i]
					if okk {
						//fmt.Println("#Server#  send data: to client", pak.msg.ConnID)
						s.listenconn.ssend(vv, v.addr)
						//	fmt.Println("write===")
						delete(v.writebuf, i)
						v.databuf[i] = vv
					}
				}
				//fmt.Println("now seq has changed to: ", c.conn.seq)

			}
		}
	}
}
func (s *server) Read() (int, []byte, error) {
	//fmt.Println("read req")
	if s.readclose == true {
		return 0, nil, errors.New("the server has closed")
	}
	packet := &Packet{nil, nil, sread}
	s.req <- packet
	x := <-s.reply
	if x.contype == -1 {
		return x.msg.ConnID, nil, errors.New("the connection is lost or ask to be closed")
	} else {
		//fmt.Println("---read---", x.msg.String())
		return x.msg.ConnID, x.msg.Payload, nil
	}
}
func (s *server) Write(connID int, payload []byte) error {
	msg := &Message{ConnID: connID, Payload: payload}
	packet := &Packet{msg, nil, swrite}
	s.req <- packet
	x := <-s.writeans
	if x.contype == -1 {
		return errors.New("#Server#  the client connection " + strconv.Itoa(connID) + " is lost when try to write")
	}
	if x.contype == 0 {
		return errors.New("#Server# the client connection " + strconv.Itoa(connID) + " is not exist when try to write")
	}
	return nil
}
func (s *server) handleWrite(connID int, data []byte) {
	msg := NewData(connID, s.sendct[connID], data)
	hold := s.sendct[connID]
	tempcon := s.conn[connID]
	if hold < tempcon.seq+tempcon.params.WindowSize {
		s.listenconn.ssend(msg, tempcon.addr)
		//fmt.Println("try to write to databuf" + msg.String())
		tempcon.databuf[hold] = msg
		//fmt.Println("#server# try to write: packet ", hold, "\tnow seq is ", tempcon.seq, " ", msg.String())

	} else {
		//fmt.Println("try to write to writebuf" + msg.String())
		tempcon.writebuf[hold] = msg
	}
	s.sendct[connID] += 1
}
func (s *server) CloseConn(connID int) error {
	fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	msg := &Message{ConnID: connID}
	packet := &Packet{msg, nil, closeconn}
	if s.readclose == false {
		s.req <- packet
		//fmt.Println("999999999")
		x := <-s.closeans
		//fmt.Println("999999999")
		if x.contype == -1 {
			return errors.New("the connid does not established")
		}
		if x.contype == -2 {
			return errors.New("the connid does not exist")
		}
		fmt.Println("close: ", connID)
		return nil
	}
	return errors.New("the server is closed give up the closeconn")
}
func (s *server) handleclosecon(pak *Packet) {
	v, _ := s.conn[pak.msg.ConnID]
	//fmt.Println("enter handle close conn: ", pak.msg.ConnID)
	if !v.losted {
		hold := &fk{nil, 0, 0, nil, nil}
		hold.databuf = make(map[int]*Message)
		s.banclose[pak.msg.ConnID] = hold
		hold.params = v.params
		hold.addr = pak.addr
		for k, v := range v.databuf {
			hold.databuf[k] = v
		}
		for k, v := range v.writebuf {
			hold.databuf[k] = v
			s.listenconn.ssend(v, hold.addr)
		}

	}
	//v.readex <- 1
	delete(s.readct, pak.msg.ConnID)
	delete(s.addmap, pak.addr.String())
	delete(s.closepend, pak.msg.ConnID)
	delete(s.conn, pak.msg.ConnID)
	delete(s.sendct, pak.msg.ConnID)
	delete(s.closeack, pak.msg.ConnID)
}
func (s *server) Close() error {
	/*
		should block until all pending messages to each client
		have been sent and acknowledged (of course, if a client
		that still has pending messages is suddenly lost during
		this time, the remaining pending messages should simply
		be discarded).
	*/
	fmt.Println("#Server# prepare to close")
	s.readclose = true
	packet := &Packet{nil, nil, sclose}
	s.req <- packet
	//fmt.Println("--------------")
	if x := <-s.over; x.contype > 0 {
		//fmt.Println("++++++++++")
		s.exitEpo <- 1
		s.exit <- 1
		s.listenconn.close()
		s.estab = false
	}
	fmt.Println("#Server# closed !!!")
	return errors.New("server closed")
}
func (s *server) handler() {
	//var now *Packet
	ct := 0
	//	fmt.Println("****")
	for {
		select {
		case <-s.exit:
			//fmt.Println("????")
			return
		default:
			ct += 1
			now := <-s.req
			//fmt.Println("****")
			switch now.contype {
			case sclose:
				//fmt.Println("!!!!!!!!!!!!!!!!", len(s.conn))
				if len(s.banclose) != 0 {
					for k, _ := range s.banclose {
						delete(s.banclose, k)
					}
				}
				flag := true
				for v, k := range s.conn {
					if k.estab {
						if k.losted == false {
							s.closepend[v] = true
						} else {
							//fmt.Println("?????")
							delete(s.addmap, k.addr.String())
							delete(s.closeack, v)
							delete(s.closepend, v)
							delete(s.conn, v)
							delete(s.sendct, v)
						}
					} else {
						if len(k.writebuf) != 0 && len(k.databuf) != 0 {
							s.closepend[v] = true
						}
					}
				}
				for v, k := range s.conn {
					if s.closepend[v] == true {
						if s.closeack[v] == true {
							//fmt.Println("?????")
							delete(s.addmap, k.addr.String())
							delete(s.closeack, v)
							delete(s.closepend, v)
							delete(s.conn, v)
							delete(s.sendct, v)
						} else {
							fmt.Println("#server# not ready to close still have: ", len(k.databuf), len(k.writebuf), len(k.ackbuf))
							flag = false
							break
						}
					}
				}
				if flag {
					ss := &Packet{nil, nil, 5}
					s.over <- ss
				}
				//s.conn.close()
			case sread:
				//fmt.Println("len: ", s.readct[1], "len(): ", len(s.conn[1].readbuf))
				for i, v := range s.conn {
					_, ok := v.readbuf[s.readct[i]]
					_, okk := s.pendconn[i]
					if !okk && ok {
						//fmt.Println("find!!!")
						//flag = true
						packet := &Packet{v.readbuf[s.readct[i]], nil, 1}
						delete(v.readbuf, s.readct[i])
						s.readct[i] += 1
						s.reply <- packet
						break
					}
				}
			case closeconn:
				v, ok := s.conn[now.msg.ConnID]
				_, ok2 := s.pendconn[now.msg.ConnID]
				//fmt.Println("wait to close ", now.msg.ConnID, " con: ", ok, "pendconn ", ok2)
				if ok == true {
					if ok2 == false {
						//msg := &Message{now.msgConnID: connID}
						packet := &Packet{now.msg, v.addr, 2}
						s.closeans <- packet
						s.handleclosecon(packet)
					} else {
						packet := &Packet{nil, nil, -1}
						s.closeans <- packet
						delete(s.addmap, v.addr.String())
						delete(s.conn, now.msg.ConnID)
						delete(s.pendconn, now.msg.ConnID)
						delete(s.sendct, now.msg.ConnID)
					}
				} else {
					packet := &Packet{nil, nil, -2}
					s.closeans <- packet
				}

				fmt.Println("#Server# ", ct, ": close client: ", now.msg.ConnID)
			case timer:
				s.handleEpoch()
			//	fmt.Println("#Server# ", ct, ": timer req")
			case receive:
				s.handleReceive(now)
				//fmt.Println("#Server# ", ct, ": receive req from client: ", now.msg.ConnID)
			case swrite:
				v, ok := s.conn[now.msg.ConnID]
				//if connID == 1 {
				//	fmt.Println("??ooooo???????")
				//}
				if ok {
					if v.losted {
						packet := &Packet{nil, nil, -1}
						s.writeans <- packet
						//return
					} else {
						packet := &Packet{nil, nil, 1}
						s.writeans <- packet
						s.handleWrite(now.msg.ConnID, now.msg.Payload)
						//fmt.Println("??????write req to: ", connID, "\t", msg.String())
					}
				} else {
					//fmt.Println("????00000000?????")
					packet := &Packet{nil, nil, 0}
					s.writeans <- packet
				}
				//fmt.Println("#Server# ", ct, ": write req to: ", now.msg.ConnID)
			}
		}
	}
}
