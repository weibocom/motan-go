package main

import (
	"fmt"

	"github.com/weibocom/motan-go"
	motancore "github.com/weibocom/motan-go/core"
)

func main() {
	runClientDemo()
}

func runClientDemo() {
	mccontext := motan.GetClientContext("main/clientdemo.yaml")
	mccontext.Start(nil)
	mclient := mccontext.GetClient("mytest-motan2")

	args := make(map[string]string, 16)
	args["name"] = "ray"
	args["id"] = "xxxx"
	var reply string
	err := mclient.Call("hello", []interface{}{args}, &reply)
	if err != nil {
		fmt.Printf("motan call fail! err:%v\n", err)
	} else {
		fmt.Printf("motan call success! reply:%s\n", reply)
	}

	// async call
	args["key"] = "test async"
	result := mclient.Go("hello", []interface{}{args}, &reply, make(chan *motancore.AsyncResult, 1))
	res := <-result.Done
	if res.Error != nil {
		fmt.Printf("motan async call fail! err:%v\n", res.Error)
	} else {
		fmt.Printf("motan async call success! reply:%+v\n", reply)
	}

	mclient2 := mccontext.GetClient("mytest-demo")
	err = mclient2.Call("hello", []interface{}{"Ray"}, &reply)
	if err != nil {
		fmt.Printf("motan call fail! err:%v\n", err)
	} else {
		fmt.Printf("motan call success! reply:%s\n", reply)
	}

	rpClient := mccontext.GetClient("reverseProxyService")
	rpArgs := map[string]string{"num1": "12345", "num2": "54321"}
	var rpReply string
	rpErr := rpClient.Call("MotanConnStr", []interface{}{rpArgs}, &rpReply)
	if rpErr != nil {
		fmt.Printf("motan call reverse proxy service fail! err:%v\n", rpErr)
	} else {
		fmt.Printf("motan call reverse proxy service success! reply:%s\n", rpReply)
	}
}
