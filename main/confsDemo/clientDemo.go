package main

import (
	"github.com/weibocom/motan-go"
	"os"
)

func main() {
	agent := motan.NewAgent(nil)
	dir, _ := os.Getwd()
	os.Args = append(os.Args, "-c", dir+"/main/confsDemo/mesh-confs", "-pool", "client_demo-yf")
	agent.StartMotanAgent()
}
