package main

/*
Scheduler Solution:
In this assignment I apply a scheduler that could equally calculate the Nonce among all the miners.
For example assuming we get a request "message 0 9999", and if we have 2 miners available,
I would distribute the workload equally that is miner1 would calculate 0 5000 while miner2 would calculate 5001 9999.
And when receiving results combining those two results to get the final answers.
During the running period, more joining miners would be moved to pending miner queue
which would be moved to the running miner queue if there is any running miner which fails before sending the results
or the current job finish.

*/
import (
	"fmt"
	"github.com/cmu440/lsp"
	//"github.com/cmu440/lspnet"
	//"bytes"
	"encoding/json"
	"github.com/cmu440/bitcoin"
	"log"
	"os"
	"strconv"
)

const (
	name = "serverlog.txt"
	flag = os.O_RDWR | os.O_CREATE | os.O_APPEND | os.O_TRUNC
	perm = os.FileMode(0666)
)

type Scheduler struct {
	pendclients map[int]int              // pend job id->client id
	waitans     chan int                 // whether current job is done
	miner       map[int]*bitcoin.Message // id->task_range_start
	pendminer   map[int]int              // new miner when the previous miner are running
	run         bool                     // whether is running jobs
	pendjobs    map[int]*bitcoin.Message //pend jobs clientid->jobs
	pendrange   map[int]*bitcoin.Message // if lost several miner in the middle block num->message
	block       int                      // block numbers
	reqct       int                      // requst number
	curclient   int                      // current client id
	currid      int                      // current job id
	receive     int                      // receive block number
	ans1        uint64                   // hash
	ans2        uint64                   // nonuce
	jobgiveup   bool                     // whether the client lost during running jobs
	minerfail   int                      // number of miner fail

}

func NewScheduler() *Scheduler {
	one := &Scheduler{
		pendclients: make(map[int]int),
		waitans:     make(chan int, 1),
		miner:       make(map[int]*bitcoin.Message),
		pendrange:   make(map[int]*bitcoin.Message),
		pendminer:   make(map[int]int),
		run:         false,
		curclient:   1,
		pendjobs:    make(map[int]*bitcoin.Message),
		block:       0,
		reqct:       1,
		currid:      1,
		ans1:        0,
		ans2:        0,
		minerfail:   0,
		receive:     0,
		jobgiveup:   false,
	}
	return one
}
func (yy *Scheduler) reassign(mlx *log.Logger, s lsp.Server) {
	if len(yy.pendrange) != 0 {

		mlx.Println("begin ressign job", yy.currid)
		for j, k := range yy.pendrange {
			for i, v := range yy.miner {
				if v == nil {
					//mlx.Println("try to reallocate lost jobs to miner", i)
					yy.miner[i] = k
					buf, _ := json.Marshal(yy.miner[i])
					errr := s.Write(i, buf)
					if errr != nil {
						delete(yy.miner, i)
					} else {
						yy.minerfail -= 1
						delete(yy.pendrange, j)
						break
					}
				}
			}
		}
	}
}
func (yy *Scheduler) resize(mlx *log.Logger, s lsp.Server) {
	if len(yy.pendminer) != 0 {
		for i, _ := range yy.pendminer {
			yy.miner[i] = nil
			delete(yy.pendminer, i)
		}
	}
}
func (yy *Scheduler) assign(mlx *log.Logger, s lsp.Server) {

	yy.jobgiveup = false
	hold := yy.pendjobs[yy.currid]
	yy.curclient = yy.pendclients[yy.currid]
	mlx.Println("begin assign job: ", yy.currid, hold.String(), " for client: ", yy.curclient, " with ", len(yy.miner), " miners")
	delete(yy.pendjobs, yy.currid)
	delete(yy.pendclients, yy.currid)
	var block uint64
	block = (hold.Upper) / uint64(len(yy.miner))
	if hold.Upper%uint64(len(yy.miner)) != 0 {
		//mlx.Println("left ", hold.Upper%uint64(len(yy.miner)))
		block += 1
	}
	yy.ans1 = 1<<64 - 1
	yy.ans2 = hold.Upper
	yy.block = len(yy.miner)
	// 41 5
	var ct uint64
	ct = 0
	fk := block + 1
	//mlx.Println("block size: ", fk)
	limit := hold.Upper
	for i, _ := range yy.miner {

		hold.Lower = ct * fk
		hold.Upper = hold.Lower + block
		if hold.Upper > limit {
			hold.Upper = limit
		}
		temp := bitcoin.NewRequest(hold.Data, hold.Lower, hold.Upper)
		mlx.Println("assign miner", i, hold.Lower, hold.Upper)
		ct += 1
		buf, _ := json.Marshal(hold)
		errr := s.Write(i, buf)
		yy.miner[i] = temp
		if errr != nil {
			mlx.Println("miner: ", i, " fail in the middle")
			yy.minerfail += 1
			yy.pendrange[i] = temp
			delete(yy.miner, i)
		}
	}
	yy.run = true
}
func main() {
	const numArgs = 2
	if len(os.Args) != numArgs {
		fmt.Println("Usage: ./server <port>")
		return
	}
	file, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return
	}
	mlx := log.New(file, "", log.Lshortfile|log.Lmicroseconds)
	port, _ := strconv.Atoi(os.Args[1])
	s, err := lsp.NewServer(port, lsp.NewParams())

	if err != nil {
		mlx.Println("cant start server in the port")
		file.Close()
		return
	}
	yy := NewScheduler()
	for {
		select {
		case <-yy.waitans:
			if yy.jobgiveup == false {
				res := bitcoin.NewResult(yy.ans1, yy.ans2)
				buf, _ := json.Marshal(res)
				mlx.Println("send ans to client: ", yy.curclient, res.String())
				errr := s.Write(yy.curclient, buf)
				if errr != nil {
					mlx.Println("server  write ans to client timeout")
				}
			}
			yy.run = false
			yy.currid += 1
			if len(yy.pendjobs) != 0 && yy.currid < yy.reqct {
				mlx.Println("there is jobs left begin to assign", yy.currid)
				mlx.Println("move pend miner to miner")
				yy.resize(mlx, s)
				mlx.Println("-----")
				yy.assign(mlx, s)
			}
		default:
			id, v, err := s.Read()
			if err != nil {
				//mlx.Println(err)
				_, ok1 := yy.miner[id]
				if ok1 {
					if yy.run {
						mlx.Println("lost miner: ", id, " when running the job")
						if yy.miner[id] != nil {
							yy.pendrange[id] = yy.miner[id]
							yy.minerfail += 1
							delete(yy.miner, id)
							yy.resize(mlx, s)
							if len(yy.miner) != 0 {
								yy.reassign(mlx, s)
							}
							continue
						}
					} else {
						mlx.Println("lost miner: ", id, " when there is no job and miner wait for request")
						delete(yy.miner, id)
						continue
					}
				}
				_, ok2 := yy.pendminer[id]
				if ok2 {
					mlx.Println("lost miner: ", id, " before assign job")
					delete(yy.pendminer, id)
					continue
				}
				if id == yy.curclient {
					mlx.Println("lost the running client: ", id, " before return results, give up current job")
					yy.jobgiveup = true
					continue
				} /*else {
					if id < yy.curclient {
						continue
					}
					mlx.Println("lost the unruning client: ", id, "before start its job")
					pos := 0
					for i, v := range yy.pendclients {
						if pos == 0 {
							if i == id {
								pos = i
								delete(yy.pendclients, i)
							}
						} else {
							if i > pos {
								delete(yy.pendclients, i)
								yy.pendclients[i-1] = v
							}
						}
					}
					for i, v := range yy.pendjobs {
						if i > pos {
							delete(yy.pendjobs, i)
							yy.pendjobs[i-1] = v
						}
					}
					yy.reqct -= 1
				}*/
			} else {
				msg := &bitcoin.Message{}
				err = json.Unmarshal(v[:], msg)
				mlx.Println(id, msg.String())
				switch msg.Type {
				case bitcoin.Join:
					if yy.run {
						yy.pendminer[id] = 0
						yy.resize(mlx, s)
						if len(yy.pendrange) != 0 {
							yy.reassign(mlx, s)
						}
					} else {
						yy.miner[id] = nil
						if len(yy.pendrange) != 0 {
							mlx.Println("request wait for miner for left job, miner come")
							yy.reassign(mlx, s)
							continue
						}
						if len(yy.pendclients) != 0 {
							mlx.Println("request wait for miner, miner come")
							yy.assign(mlx, s)
						}
					}
				case bitcoin.Request:
					mlx.Println("receive request form client:", id, msg.String())
					if yy.run {
						yy.pendclients[yy.reqct] = id
						yy.pendjobs[yy.reqct] = msg
						yy.reqct += 1
					} else {
						if len(yy.miner) != 0 {
							yy.pendclients[yy.reqct] = id
							yy.pendjobs[yy.reqct] = msg
							yy.reqct += 1
							mlx.Println("miner wait for request, client come")

							yy.assign(mlx, s)
						} else {
							yy.pendclients[yy.reqct] = id
							yy.pendjobs[yy.reqct] = msg
							yy.reqct += 1
						}
					}
				case bitcoin.Result:
					mlx.Println("receive ans: ", msg.Hash, msg.Nonce)
					if yy.run {
						for i, v := range yy.miner {
							if v == nil {
								continue
							}
							mlx.Println("check miner: ", i, v.Lower, v.Upper)
							if v != nil && msg.Nonce >= v.Lower && msg.Nonce <= v.Upper {
								if msg.Hash < yy.ans1 {
									yy.ans1 = msg.Hash
									yy.ans2 = msg.Nonce
								} else if msg.Hash == yy.ans1 {
									if msg.Nonce < yy.ans2 {
										yy.ans2 = msg.Nonce
									}
								}
								yy.miner[i] = nil
								yy.block -= 1
								mlx.Println("left ", yy.block, " answers, now answer: ", yy.ans1, yy.ans2)
								break
							}
						}
						if yy.block == 0 {
							//yy.ans1 = msg.Hash
							//yy.ans2 = msg.Nonce
							yy.waitans <- 1
							continue
						}
						if yy.minerfail > 0 {
							yy.reassign(mlx, s)
						}
					}
				}
			}

		}
	}
}
