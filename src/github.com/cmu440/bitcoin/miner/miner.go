package main

import (
	"fmt"
	"github.com/cmu440/lsp"
	//"github.com/cmu440/lspnet"
	//"bytes"
	"encoding/json"
	"github.com/cmu440/bitcoin"
	"log"
	"os"
)

const (
	name = "minerlog.txt"
	flag = os.O_RDWR | os.O_CREATE | os.O_APPEND | os.O_TRUNC
	perm = os.FileMode(0666)
)

func main() {
	const numArgs = 2
	if len(os.Args) != numArgs {
		fmt.Println("Usage: ./miner <hostport>")
		return
	}

	file, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return
	}

	mlx := log.New(file, "", log.Lshortfile|log.Lmicroseconds)

	min, err := lsp.NewClient(os.Args[1], lsp.NewParams())

	if err != nil {
		mlx.Println("cant connect the server")
		file.Close()
		return
	}
	msg := bitcoin.NewJoin()
	buf, _ := json.Marshal(msg)
	hold := min.ConnID()
	mlx.Println("get id: ", hold)
	errr := min.Write(buf)
	if errr != nil {
		mlx.Println("write join fail")
		min.Close()
		file.Close()
		return
	} else {
		mlx.Println("miner: ", hold, " write join done")
		mlx.Println(msg.String())
	}
	for {
		v, er := min.Read()
		//mlx.Println("+++++")
		if er != nil {
			mlx.Println(er)
			file.Close()
			//min.Close()
			return
		}
		msgg := &bitcoin.Message{}

		err = json.Unmarshal(v[:], msgg)
		mlx.Println("miner: ", hold, " read request->", msgg.String())
		mes := msgg.Data
		l := msgg.Lower
		r := msgg.Upper
		var hold, ans1, ans2 uint64
		ans2 = 1<<64 - 1
		ans1 = 1<<64 - 1
		for i := l; i <= r; i++ {
			hold = bitcoin.Hash(mes, i)
			if hold < ans1 {
				ans1 = hold
				ans2 = i
			} else if hold == ans1 {
				if i < ans2 {
					ans2 = i
				}
			}
		}
		res := bitcoin.NewResult(ans1, ans2)
		buf, _ := json.Marshal(res)
		errr := min.Write(buf)
		if errr != nil {
			mlx.Println("miner: ,", hold, " write ans timeout fail")
			min.Close()
			file.Close()
			return
		} else {
			mlx.Println("miner: ", hold, " write ans done")
			mlx.Println(msg.String())
		}
	}
}
