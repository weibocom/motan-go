package motan

import (
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/weibocom/motan-go/cluster"
	"github.com/weibocom/motan-go/core"
	mhttp "github.com/weibocom/motan-go/http"
)

const (
	goNum      = 5
	requestNum = 10000
)

var proxyClient *http.Client

func TestHTTPProxy(t *testing.T) {
	wg := &sync.WaitGroup{}
	wg.Add(goNum)
	requests := make(chan int, requestNum)
	for i := 0; i < goNum; i++ {
		go func() {
			defer wg.Done()
			for req := range requests {
				suffix := "test" + strconv.Itoa(req)
				resp, err := proxyClient.Get("http://test.domain/tst/test?index=" + suffix)
				if err != nil {
					continue
				}
				bytes, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					resp.Body.Close()
					continue
				}
				resp.Body.Close()
				if !strings.HasSuffix(string(bytes), suffix) {
					t.Errorf("wrong response")
				}
			}
		}()
	}
	for i := 0; i < requestNum; i++ {
		requests <- i
	}
	close(requests)
	wg.Wait()
}

func TestRpcToHTTPProxy(t *testing.T) {
	context := &core.Context{}
	context.RegistryURLs = make(map[string]*core.URL)
	context.ClientURL = &core.URL{}
	context.ClientURL.PutParam(core.ApplicationKey, "motan-test-client")

	extFactory := GetDefaultExtFactory()
	registryURL := &core.URL{Protocol: "direct", Host: "127.0.0.1", Port: 9981}
	context.RegistryURLs["direct-registry"] = registryURL

	clientURL := &core.URL{}
	clientURL.Protocol = "motan2"
	clientURL.Path = "test.domain"
	clientURL.PutParam(core.RegistryKey, "direct-registry")
	clientURL.PutParam(core.TimeOutKey, "100000")
	clientCluster := cluster.NewCluster(context, extFactory, clientURL, false)
	client := Client{url: clientURL, cluster: clientCluster, extFactory: extFactory}
	request := client.BuildRequest("/tst/xxxx/111", []interface{}{map[string]string{"a": "a"}})
	var reply []byte
	client.BaseCall(request, &reply)
	assert.Equal(t, "/2/tst/xxxx/111?a=a", string(reply))
	request.SetAttachment(mhttp.QueryString, "b=b")
	request.SetAttachment(mhttp.Method, "POST")
	client.BaseCall(request, &reply)
	assert.Equal(t, "/2/tst/xxxx/111?b=b", string(reply))

	wg := &sync.WaitGroup{}
	wg.Add(goNum)
	requests := make(chan int, requestNum)
	for i := 0; i < goNum; i++ {
		go func() {
			defer wg.Done()
			for req := range requests {
				suffix := "test" + strconv.Itoa(req)
				request := client.BuildRequest("/tst/test", []interface{}{map[string]string{"index=": suffix}})
				request.SetAttachment(mhttp.Method, "GET")
				var reply []byte
				client.BaseCall(request, &reply)
				if !strings.HasSuffix(string(reply), suffix) {
					t.Errorf("wrong response")
				}
			}
		}()
	}
	for i := 0; i < requestNum; i++ {
		requests <- i
	}
	close(requests)
	wg.Wait()

}

func TestMain(m *testing.M) {
	cfgFile := filepath.Join("testdata", "agent.yaml")
	go func() {
		var addr = ":9090"
		handler := &http.ServeMux{}
		handler.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			request.ParseForm()
			writer.Write([]byte(request.URL.String()))
		})
		http.ListenAndServe(addr, handler)
	}()
	go func() {
		agent := NewAgent(nil)
		agent.ConfigFile = cfgFile
		agent.StartMotanAgent()
	}()
	proxyURL, _ := url.Parse("http://localhost:9983")
	proxyClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	time.Sleep(1 * time.Second)
	for i := 0; i < 100; i++ {
		resp, err := proxyClient.Get("http://test.domain/tst/test")
		if err != nil {
			continue
		}
		if resp.StatusCode != 200 {
			continue
		}
		time.Sleep(1 * time.Second)
		break
	}
	os.Exit(m.Run())
}
