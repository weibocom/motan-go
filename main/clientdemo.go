package main

import (
	"fmt"

	"github.com/weibocom/motan-go"
)

func main() {
	runClientDemo()
}

func runClientDemo() {
	mccontext := motan.GetClientContext("main/clientdemo.yaml")
	mccontext.Start(nil)

	//// sync call
	//mclient := mccontext.GetClient("mytest-motan2")
	//args := make(map[string]string, 16)
	//args["name"] = "ray"
	//args["id"] = "xxxx"
	//var reply string
	//err := mclient.Call("hello", []interface{}{args}, &reply)
	//if err != nil {
	//	fmt.Printf("motan call fail! err:%v\n", err)
	//} else {
	//	fmt.Printf("motan call success! reply:%s\n", reply)
	//}

	//mclient2 := mccontext.GetClient("mytest-demo")
	//err = mclient2.Call("hello", []interface{}{"Ray"}, &reply)
	//if err != nil {
	//	fmt.Printf("motan call fail! err:%v\n", err)
	//} else {
	//	fmt.Printf("motan call success! reply:%s\n", reply)
	//}

	//// async call
	//args["key"] = "test async"
	//result := mclient.Go("hello", []interface{}{args}, &reply, make(chan *motancore.AsyncResult, 1))
	//res := <-result.Done
	//if res.Error != nil {
	//	fmt.Printf("motan async call fail! err:%v\n", res.Error)
	//} else {
	//	fmt.Printf("motan async call success! reply:%+v\n", reply)
	//}

	//zha测试
	//zha1Client := mccontext.GetClient("httptest-zha")
	//for {
	zha0Client := mccontext.GetClient("motantest-zha0")
	zha0ArgsStr := map[string]string{"num1": "zha", "num2": "0000"}
	var reply0 string
	err0 := zha0Client.Call("MotanAddStr", []interface{}{zha0ArgsStr}, &reply0)
	if err0 != nil {
		fmt.Printf("motan call0000 fail! err:%v\n", err0)
	} else {
		fmt.Printf("motan call0000 success! reply:%s\n", reply0)
	}

	zha1Client := mccontext.GetClient("motantest-zha1")
	zha1ArgsStr := map[string]string{"num1": "zha", "num2": "1111"}
	var reply1 string
	err1 := zha1Client.Call("MotanAddStr", []interface{}{zha1ArgsStr}, &reply1)
	if err1 != nil {
		fmt.Printf("motan call1111 fail! err:%v\n", err1)
	} else {
		fmt.Printf("motan call1111 success! reply:%s\n", reply1)
	}
	//}
}
