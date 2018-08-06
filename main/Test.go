package main

import (
	"fmt"
	"math/rand"
	"strconv"
)

func main() {
	for i := 0; i < 100; i++ {
		n := rand.Intn(10)
		if n == 0 {
			fmt.Println(strconv.Itoa(i) + ":" + strconv.Itoa(n))
		}

		//test(i)
	}
}

func test(i int) {
	defer func() {
		if err := recover(); err != nil {
			fmt.Printf("---'%d', %v \n", i, err)
		}

	}()
	panic("xx" + strconv.Itoa(i))
}
