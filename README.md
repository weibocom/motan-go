# Motan-go
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://github.com/weibocom/motan/blob/master/LICENSE)
[![Build Status](https://img.shields.io/travis/weibocom/motan-go/master.svg?label=Build)](https://travis-ci.org/weibocom/motan-go)

# Overview
[Motan][motan] is a cross-language remote procedure call(RPC) framework for rapid development of high performance distributed services.

This project is the golang Motan implementation. Provides golang motan server, motan client and motan agent. 
motan agent is designed to support bio-proxy for any other language such as PHP, Python by motan2 protocol.

# Features
- Interactive with mulit language through motan2 protocol,such as Java, PHP.
- Provides cluster support and integrate with popular service discovery services like [Consul][consul] or [Zookeeper][zookeeper]. 
- Supports advanced scheduling features like weighted load-balance, scheduling cross IDCs, etc.
- Optimization for high load scenarios, provides high availability in production environment.
- Supports both synchronous and asynchronous calls.

# Quick Start

## Installation

```sh
go get -u -v github.com/weibocom/motan-go
```

The quick start gives very basic example of running client and server on the same machine. For the detailed information about using and developing Motan, please jump to [Documents](#documents).
the demo case is in the main/ directory

## Motan server

1. Create serverdemo.yaml to config service

```yaml
#config of registries
motan-registry:
  direct-registry: # registry id 
    protocol: direct   # registry type

#conf of services
motan-service:
  mytest-motan2:
    path: com.weibo.motan.demo.service.MotanDemoService # e.g. service name for register
    group: motan-demo-rpc
    protocol: motan2
    registry: direct-registry
    serialization: simple
    ref : "main.MotanDemoService"
    export: "motan2:8100"
```

2. Write an implementation, create and start RPC Server.

```go
package main

import (
	"fmt"
	"time"

	motan "github.com/weibocom/motan-go"
)

func main() {
	runServerDemo()
}

func runServerDemo() {
	mscontext := motan.GetMotanServerContext("serverdemo.yaml") //get config by filename
	mscontext.RegisterService(&MotanDemoService{}, "") // registry implement
	mscontext.Start(nil) // start server
	time.Sleep(time.Second * 50000000)
}

// service implement
type MotanDemoService struct{}

func (m *MotanDemoService) Hello(name string) string {
	fmt.Printf("MotanDemoService hello:%s\n", name)
	return "hello " + name
}
```

## Motan client

1. Create clientdemo.yaml to config service for subscribe

```yaml
#config of registries
motan-registry:
  direct-registry: # registry id 
    protocol: direct   # registry type. 
    host: 127.0.0.1 
    port: 9981 

#conf of refers
motan-refer:
  mytest-motan2:
    path: com.weibo.motan.demo.service.MotanDemoService # e.g. service name for subscribe
    group: motan-demo-rpc # group name
    protocol: motan2 # rpc protocol
    registry: direct-registry
    requestTimeout: 1000
    serialization: simple
    haStrategy: failover
    loadbalance: roundrobin
```

2. Start call

```go
package main

import (
	"fmt"

	motan "github.com/weibocom/motan-go"
	motancore "github.com/weibocom/motan-go/core"
)

func main() {
	runClientDemo()
}

func runClientDemo() {
	mccontext := motan.GetMotanClientContext("clientdemo.yaml")
	mccontext.Start(nil)
	mclient := mccontext.GetClient("mytest-motan2")

	var reply string
	err := mclient.Call("hello", "Ray", &reply)  // sync call
	if err != nil {
		fmt.Printf("motan call fail! err:%v\n", err)
	} else {
		fmt.Printf("motan call success! reply:%s\n", reply)
	}

	// async call
	result := mclient.Go("hello", "Ray", &reply, make(chan *motancore.AsyncResult, 1))
	res := <-result.Done
	if res.Error != nil {
		fmt.Printf("motan async call fail! err:%v\n", res.Error)
	} else {
		fmt.Printf("motan async call success! reply:%+v\n", reply)
	}
}

```

## Use agent. 

agent is not necessary for golang. it designed for interpreted languages such as PHP to support service governance

1. Create clientdemo.yaml to config service for subscribe or register

```yaml
#config fo agent
motan-agent:
  port: 9981 # agent serve port. 
  mport: 8002 # agent manage port 

#config of registries
motan-registry:
  direct-registry: # registry id 
    protocol: direct   # registry type. will get instance from extFactory.
    host: 127.0.0.1 # direct server ip.
    port: 8100 #direct server port

#conf of refers
motan-refer:
  mytest-motan2:
    path: com.weibo.motan.demo.service.MotanDemoService # e.g. service name for subscribe
    group: motan-demo-rpc
    protocol: motan2
    registry: direct-registry
    serialization: simple
```

2. Start Agent

```go
package main

import motan "github.com/weibocom/motan-go"

func main() {
	runAgentDemo()
}

func runAgentDemo() {
	agent := motan.NewAgent(nil)
	agent.ConfigFile = "./agentdemo.yaml"
	agent.StartMotanAgent()
}
```

# Documents

TBD

# Contributors

* Ray([@rayzhang0603](https://github.com/rayzhang0603))
* 周晶([@idevz](https://github.com/idevz))
* xiaohutuer([@xiaohutuer](https://github.com/xiaohutuer))
* Arthur Guo([@jealone](https://github.com/jealone))
* huzhongx([@huzhongx](https://github.com/huzhongx))

# License

Motan is released under the [Apache License 2.0](http://www.apache.org/licenses/LICENSE-2.0).

[motan]:https://github.com/weibocom/motan
[consul]:http://www.consul.io
[zookeeper]:http://zookeeper.apache.org