package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"

	"github.com/weibocom/motan-go"
	motancore "github.com/weibocom/motan-go/core"
)

func main() {
	go func() {
		var addr = ":9090"
		handler := &http.ServeMux{}
		handler.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Add("server-address", motancore.GetLocalIP()+addr)
			_, _ = fmt.Fprint(writer, "request url: "+request.URL.String()+"\n")
			rawRequestBytes, err := httputil.DumpRequest(request, true)
			if err != nil {
				http.Error(writer, fmt.Sprint(err), http.StatusInternalServerError)
				return
			} else {
				fmt.Println(string(rawRequestBytes))
			}
		})
		http.ListenAndServe(addr, handler)
	}()
	motan.PermissionCheck = func(r *http.Request) bool {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		if host == "127.0.0.1" || host == "::1" {
			return true
		}
		return false
	}

	dir, _ := os.Getwd()
	os.Args = append(os.Args, "-c", dir+"/main/confsDemo/mesh-confs", "-pool", "server_demo-yf")
	agent := motan.NewAgent(nil)
	agent.StartMotanAgent()
}
