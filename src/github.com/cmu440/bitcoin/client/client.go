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
	"strconv"
	//"strings"
)

const (
	name = "clientlog.txt"
	flag = os.O_RDWR | os.O_CREATE | os.O_APPEND | os.O_TRUNC
	perm = os.FileMode(0666)
)

func main() {
	const numArgs = 4
	if len(os.Args) != numArgs {
		fmt.Println("Usage: ./client <hostport> <message> <maxNonce>")
		return
	}

	file, err := os.OpenFile(name, flag, perm)
	if err != nil {
		return
	}

	mlx := log.New(file, "", log.Lshortfile|log.Lmicroseconds)

	cli, err := lsp.NewClient(os.Args[1], lsp.NewParams())

	if err != nil {
		mlx.Println("cant connect the server")
		file.Close()
		return
	}

	//mlx.Println("cant connect the server")
	//fmt.Println(parms.WindowSize)
	mlx.Println("req: ", os.Args[0], os.Args[1], os.Args[2], os.Args[3])
	na, _ := strconv.ParseUint(os.Args[3], 10, 64)
	msg := bitcoin.NewRequest(os.Args[2], 0, na)
	buf, _ := json.Marshal(msg)
	hold := cli.ConnID()
	mlx.Println("get id: ", hold)
	errr := cli.Write(buf)
	if errr != nil {
		printDisconnected()
		mlx.Println("client: ", hold, "write fail")
		cli.Close()
		file.Close()
		return
	} else {
		mlx.Println("client: ", hold, " write done")
		mlx.Println(msg.String())
	}
	v, er := cli.Read()
	//mlx.Println("+++++")
	if er != nil {
		printDisconnected()
		mlx.Println(er)
		file.Close()
		//cli.Close()
	} else {
		//n := bytes.Index(v, []byte{0})
		msgg := &bitcoin.Message{}

		err = json.Unmarshal(v[:], msgg)
		mlx.Println("client: ", hold, " read->", msgg.String())
		printResult(strconv.FormatUint(msgg.Hash, 10), strconv.FormatUint(msgg.Nonce, 10))
		cli.Close()
		file.Close()
	}

}

// printResult prints the final result to stdout.
func printResult(hash, nonce string) {
	fmt.Println("Result", hash, nonce)
}

// printDisconnected prints a disconnected message to stdout.
func printDisconnected() {
	fmt.Println("Disconnected")
}
