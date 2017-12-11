package main

import (
	"fmt"

	motan "github.com/weibocom/motan-go"
)

func main() {
	runClientDemo()
}

func runClientDemo() {
	mccontext := motan.GetClientContext("./clientdemo-json.yaml")
	mccontext.Start(nil)

	mclient2 := mccontext.GetClient("mytest-demo")
	type People struct {
		Name string
		Age  int
	}

	var reply string
	args := []interface{}{&People{Name: "Ray"}, &People{Age: 18}}
	err := mclient2.Call("hello", args, &reply)
	if err != nil {
		fmt.Printf("motan call fail! err:%v\n", err)
	} else {
		fmt.Printf("motan call success! reply:%s\n", reply)
	}

}
