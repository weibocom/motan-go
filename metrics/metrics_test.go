package metrics

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	metrics "github.com/rcrowley/go-metrics"
)

const (
	testWriter = "test"
)

var (
	count int64
)

func runTest(typ string) error {
	if "graphite" == typ {
		// var m metric
		m := metric{
			Period: 5,
			Graphite: []graphite{graphite{
				Host: "log.monitor.weibo.com",
				Port: 8333,
				Name: "com.weibo.api.service.StatusesRpc"}}}

		for _, g := range m.Graphite {
			w, err := getWriter(&g)
			if err != nil {
				return err
			}
			reg.writers[g.Name] = w
		}
	} else {
		reg.writers[testWriter] = new(testMetrics)
	}
	go reg.eventLoop()
	return nil
}

type testMetrics struct {
}

func (t *testMetrics) Write(snap metrics.Registry) error {
	ip := "127.0.0.1"
	ip = strings.Replace(ip, ".", "_", -1)
	messages := genGraphiteMessages(ip, snap)
	fmt.Println(snap)
	// fmt.Printf("%s, %+v", "success", snap)
	for _, message := range messages {
		v := strings.SplitN(message, ":", 2)
		number := strings.SplitN(v[1], "|", 2)
		times, err := strconv.ParseInt(number[0], 10, 0)
		if nil == err {
			count += times
		}
		fmt.Printf("%s", message)
	}
	return nil
}

//TODO 修改
func TestCounter(T *testing.T) {
	// count = 0
	// var testCount int64 = 1000
	// // 切换走upd的graphite还是本地标准输出
	// // runTest("graphite")
	// runTest("locate")
	// key := fmt.Sprintf("%s:%s:%s:%s", "M_s", "M_g", "M_p", "Method")
	// key2 := fmt.Sprintf("%s:%s:%s:%s", "M_s", "M_g", "M_p", "Method_2")
	// // keyCount := key + ".total_count"
	// // keyAvgTime := key + ".avg_time"

	// go func(key string) {
	// 	for {
	// 		AddHistograms(key, 100)
	// 		AddCounter(key+".total_count", 10)
	// 		time.Sleep(time.Second * 1)
	// 	}
	// }(key)

	// go func(key string) {
	// 	for {
	// 		AddCounter(key+".total_count", 50)
	// 		AddHistograms(key, 10)
	// 		time.Sleep(time.Second * 1)
	// 	}
	// }(key2)

	// // for i = 1; i <= testCount; i++ {
	// // 	AddCounter(keyCount, 1)
	// // 	time.Sleep(time.Millisecond * 10)
	// // }
	// time.Sleep(10 * time.Second)
	// if count != testCount {
	// 	T.Errorf("%d times count but %d times record\n", testCount, count)
	// } else {
	// 	T.Logf("Counter of Metrics work properly\n")
	// }
}
