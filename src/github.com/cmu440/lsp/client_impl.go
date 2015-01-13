// Contains the implementation of a LSP client.

package lsp

import (
	"errors"
	"fmt"
	"github.com/cmu440/lspnet"
	"strconv"
)

type client struct {
	initparams  *Params
	req         chan *Packet
	reply       chan *Packet
	over        chan *Packet
	conn        *Connect
	readlost    chan int
	readclose   chan int
	replyread   chan *Packet
	conId       int
	exit        chan int
	closepend   bool
	readall     chan *Message
	closeack    bool
	sendct      int
	readct      int
	exitEpo     chan int
	readnum     int
	lastreadnum int
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	c := &client{
		initparams:  params,
		req:         make(chan *Packet, 1000),
		reply:       make(chan *Packet, 100),
		over:        make(chan *Packet, 1000),
		conId:       0,
		sendct:      1,
		readct:      1,
		replyread:   make(chan *Packet, 100),
		closepend:   false,
		closeack:    false,
		readall:     make(chan *Message, 10),
		readlost:    make(chan int, 1),
		readclose:   make(chan int, 1),
		exit:        make(chan int, 1),
		exitEpo:     make(chan int, 1),
		readnum:     0,
		lastreadnum: 0,
	}
	c.conn = NewConnection(c.req, c.initparams)
	//fmt.Println("++++")
	err := c.conn.dial(hostport)
	if err != nil {
		return nil, err
	}
	//fmt.Println("------")
	go c.handler()
	go c.conn.connectHandler(0)
	go Timers(c.req, c.exitEpo, c.conn.params.EpochMillis)
	//fmt.Println("++++")
	msg := &Message{}
	packet := &Packet{msg, nil, cconnects}
	c.req <- packet
	select {
	case <-c.conn.lost:
		c.exitEpo <- 1
		c.exit <- 1
		c.conn.close()
		return nil, errors.New("connection is lost during establishment")
	case <-c.conn.build:
		//fmt.Println("#Client# connection established ID: ", c.conId)
		// send real data from seq number 1
		//c.conn.seq += 1
		c.conn.lastrece = c.conn.receive
		c.conn.estab = true
		return c, nil
	}
}
func (now *Connect) dial(hostport string) error {
	serverAddr, err := lspnet.ResolveUDPAddr("udp", hostport)
	if err != nil {
		return err
	}
	conn, err := lspnet.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return err
	}
	now.addr = serverAddr
	now.conn = conn
	return nil
}
func (c *client) ConnID() int {
	if c.conn.estab == false {
		return 0
	} else {
		msg := &Message{}
		packet := &Packet{msg, nil, connectid}
		//	fmt.Println("check ID")
		c.req <- packet
		//	fmt.Println("......")
		if x := <-c.reply; x.contype > 0 {
			//fmt.Println("#client# return id: ", x.contype)
			return x.contype
		} else {
			fmt.Println("error about the ConnID")
			return 0
		}
	}
}
func (c *client) Close() error {
	/*
		should not forcefully terminate the connection,
		but instead should block until all pending messages to
		the server have been sent and acknowledged (of course,
		if the connection is suddenly lost during this time,
		the remaining pending messages should simply be discarded).
	*/
	fmt.Println("#Client# prepare to close ", c.conId)
	packet := &Packet{nil, nil, cclose}
	c.req <- packet
	if x := <-c.over; x.contype > 0 {
		c.conn.close()
		c.exitEpo <- 1
		c.exit <- 1
		//c.conn.readex <- 1
		c.conn.estab = false
		//fmt.Println("cccccc")
	}
	return errors.New("#Client# client " + strconv.Itoa(c.conId) + "closed")
}
func (c *client) handleEstab() {
	msg := NewConnect()
	c.conn.send(msg)
}
func (c *client) handleEpoch() {
	// establised but no message received

	//fmt.Println("#client# ", c.conId, " no message send connect to keep connection")

	if c.conId == 0 {
		c.conn.lastrece = c.conn.receive
		hold := NewConnect()
		c.conn.send(hold)
		return
	}
	hold := NewAck(c.conId, 0)
	//c.conn.lastrece = c.conn.receive
	c.conn.send(hold)

	//c.lastreadnum = c.readnum
	//fmt.Println("client tiem()()(): ", c.conId, c.conn.receive+1, c.conn.lastrece, c.conn.params.EpochLimit, len(c.conn.ackbuf), len(c.conn.databuf))
	c.conn.receive += 1
	if c.conn.receive-c.conn.lastrece >= c.conn.params.EpochLimit {
		if c.conId == 0 {
			// lost before established
			fmt.Println("#Client# lost server before establish")
			c.conn.lost <- 1
			return
		} else {
			fmt.Println("#Client# lost server in the middle")
			c.conn.losted = true
			packet := &Packet{nil, nil, -2}
			c.replyread <- packet
			if c.closepend == true {
				c.closeack = true
				packet := &Packet{nil, nil, cclose}
				c.req <- packet
			} else {
				ss := &Packet{nil, nil, 5}
				c.over <- ss
				c.Close()
			}
			//return
		}
	}
	// timeout resend establish message

	//resend last acked data in case the server did not receive
	for _, temp := range c.conn.ackbuf {
		//fmt.Println("#Client# rsend ack: ", temp.SeqNum)
		c.conn.send(temp)
	}
	//resend those data buf in case that the server did not send acked
	for _, temp := range c.conn.databuf {
		//fmt.Println("#Client# rsend package: ", temp.SeqNum)
		c.conn.send(temp)
	}
}
func (c *client) handleReceive(msg *Message) {
	//fmt.Println("#client# ", c.conId, " receive ", msg.String())
	if msg.Type == MsgConnect {
		//fmt.Println("#client# error with MSgconnect")
		return
	}
	if c.closepend == true && len(c.conn.writebuf) == 0 && len(c.conn.databuf) == 0 {
		c.closeack = true
		packet := &Packet{nil, nil, cclose}
		c.req <- packet
		fmt.Println("wait for close: ", c.conId)
		return
	}
	if c.closepend == true && c.conn.losted == true {
		c.closeack = true
		packet := &Packet{nil, nil, cclose}
		c.req <- packet
		fmt.Println("#client# find lost to close")
		return
	}
	if c.closepend == true && msg.Type != MsgAck {
		//fmt.Println("shifou")
		return
	}
	c.readnum += 1
	switch msg.Type {
	case MsgAck:
		//fmt.Println("receive msgack: ", msg.SeqNum)
		if c.conId == 0 && msg.SeqNum == 0 && c.conn.estab == false {
			c.conId = msg.ConnID
			c.conn.build <- 1
			c.conn.seq += 1
			//c.conn.lastrece = c.conn.receive
			//	fmt.Println("-----")
			return
		}
		if c.conId > 0 && msg.SeqNum == 0 && c.conn.estab == true {
			c.conn.lastrece = c.conn.receive
			fmt.Println("------server ask not closed-------")
			return
		}
		//	fmt.Println("#client# ", c.conId, " ack: ", msg.SeqNum, c.conn.seq)
		if msg.SeqNum >= c.conn.seq && msg.SeqNum <= c.conn.seq+c.conn.params.WindowSize {
			c.conn.receive = c.conn.lastrece
			delete(c.conn.databuf, msg.SeqNum)
			if msg.SeqNum == c.conn.seq {

				hold := c.conn.seq
				c.conn.seq += 1
				ct := 1
				//find the maximum window sliding
				for i := c.conn.seq; len(c.conn.databuf) != 0 && i < hold+c.conn.params.WindowSize-1; i++ {
					_, ok := c.conn.databuf[i]
					if ok {
						break
					} else {
						c.conn.seq += 1
						ct += 1
					}
				}
				// because the sliding try to send the maximum new data
				for i := hold + c.conn.params.WindowSize; len(c.conn.writebuf) != 0 && i < hold+c.conn.params.WindowSize+ct; i++ {
					v, ok := c.conn.writebuf[i]
					if ok {
						//fmt.Println("#Client# send data: ", i)
						c.conn.send(v)
						delete(c.conn.writebuf, i)
						c.conn.databuf[i] = v
					}
				}
				//fmt.Println("now seq has changed to: ", c.conn.seq)

			}
		}
	case MsgData:
		/*
			if c.conId > 0 && msg.SeqNum > 0 && c.conn.estab == false {
				c.conn.build <- 1
				//c.conn.lastrece = c.conn.receive
				//	fmt.Println("-----")
				return
			}
		*/
		fmt.Println("#client# ", c.conId, " receive msgdata: ", msg.SeqNum, "now ack: ", c.conn.ack)
		if msg.SeqNum >= c.conn.ack && c.conn.losted == false && c.conn.estab == true {
			c.conn.receive = c.conn.lastrece
			ackk := NewAck(c.conId, msg.SeqNum)
			c.conn.send(ackk)
			fmt.Println("#client# ", c.conId, " send ack to acknowledge: ", msg.SeqNum)
			_, ex := c.conn.ackbuf[msg.SeqNum]
			c.conn.ackbuf[msg.SeqNum] = ackk
			if msg.SeqNum-c.conn.params.WindowSize >= c.conn.ack {
				for i := c.conn.ack; i <= msg.SeqNum-c.conn.params.WindowSize; i++ {
					delete(c.conn.ackbuf, i)
				}
				c.conn.ack = msg.SeqNum - c.conn.params.WindowSize + 1
			}
			//fmt.Println("@@@@@@@@@@@@@@", msg.SeqNum, c.conn.ack)
			if !ex {
				c.conn.readbuf[msg.SeqNum] = msg
				packet := &Packet{nil, nil, cread}
				c.req <- packet
			}
			//fmt.Println("now ack has changed to: ", c.conn.ack)
		}
	}
}
func (c *client) Read() ([]byte, error) {
	//fmt.Println("need read")
	if c.closepend == true {
		return nil, errors.New("connection is going to close before read")
	}
	packet := &Packet{nil, nil, cread}
	c.req <- packet
	//fmt.Println("-----")
	x := <-c.replyread
	if x.contype == -1 {
		return nil, errors.New("connection closed during read")
	} else if x.contype == -2 {
		return nil, errors.New("connection lost during read")
	} else {
		//	fmt.Println("---client read---", x.msg.String())
		return x.msg.Payload, nil
	}
}
func (c *client) Write(payload []byte) error {
	//fmt.Println("client wrtie")
	msg := &Message{Payload: payload}
	packet := &Packet{msg, nil, cwrite}
	c.req <- packet
	if c.conn.losted == true {
		return errors.New("connection has lost before trying to write")
	} else {
		return nil
	}
}
func (c *client) handleWrite(data []byte) {
	msg := NewData(c.conId, c.sendct, data)

	if c.sendct < c.conn.seq+c.conn.params.WindowSize {
		c.conn.send(msg)
		//fmt.Println("!!!!!client", c.conId, " try to write"+msg.String())
		c.conn.databuf[c.sendct] = msg
	} else {
		c.conn.writebuf[c.sendct] = msg
	}
	c.sendct += 1
}
func (c *client) handler() {
	//var now *Packet
	ct := 0

	for {
		select {
		case <-c.exit:
			//	fmt.Println("????")
			return
		default:
			ct += 1
			now := <-c.req
			//	fmt.Println("****")
			switch now.contype {
			case cconnects:
				c.handleEstab()
				//fmt.Println("#Client# ", ct, ": connect req")
			case cread:
				//	fmt.Println("#client# read req: ", len(c.conn.readbuf))
				_, ok := c.conn.readbuf[c.readct]
				if ok {
					//	fmt.Println("find")
					packet := &Packet{c.conn.readbuf[c.readct], nil, 1}
					delete(c.conn.readbuf, c.readct)
					c.readct += 1
					c.replyread <- packet
				}

			case connectid:
				//	fmt.Println("#Client# ", ct, ": connect id req")
				packet := &Packet{contype: c.conId}
				c.reply <- packet
			//	fmt.Println("id done")
			case timer:
				fmt.Println("#Client# ", ct, ": timer req")
				c.handleEpoch()

			case cclose:
				if c.conn.estab == true {
					c.closepend = true
					if c.conn.losted == false {
						if c.closeack == true {
							ss := &Packet{nil, nil, 5}
							c.over <- ss
						} else {
							fmt.Println("still has ", len(c.conn.writebuf), len(c.conn.databuf), len(c.conn.ackbuf))
						}
					} else {
						ss := &Packet{nil, nil, 5}
						c.over <- ss
					}

				}

			case receive:
				//	fmt.Println("#Client# ", ct, ": receive req")
				c.handleReceive(now.msg)
			//	fmt.Println("#Client#  receive done")

			case cwrite:
				//	fmt.Println("#Client# ", ct, ": write req: ", now.msg.String())
				c.handleWrite(now.msg.Payload)

			}
		}
	}

}
