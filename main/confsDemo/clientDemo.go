package main

import (
	"os"

	"github.com/weibocom/motan-go"
)

func main() {
	agent := motan.NewAgent(nil)
	dir, _ := os.Getwd()
	os.Args = append(os.Args, "-c", dir+"/main/confsDemo/mesh-confs", "-pool", "client_demo-yf")
	agent.StartMotanAgent()
}
