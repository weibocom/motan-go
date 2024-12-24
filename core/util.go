package core

import (
	"bytes"
	"math/rand"
	"net"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/weibocom/motan-go/log"
)

const (
	defaultServerPort = "9982"
	defaultProtocol   = "motan2"
)

var (
	PanicStatFunc func()

	localIPs      = make([]string, 0)
	initDirectEnv sync.Once
	directRpc     map[string]*URL
)

func ParseExportInfo(export string) (string, int, error) {
	port := defaultServerPort
	protocol := defaultProtocol
	s := TrimSplit(export, ":")
	if len(s) == 1 && s[0] != "" {
		port = s[0]
	} else if len(s) == 2 {
		if s[0] != "" {
			protocol = s[0]
		}
		port = s[1]
	}
	pi, err := strconv.Atoi(port)
	if err != nil {
		vlog.Errorf("export port not int. port:%s", port)
		return protocol, pi, err
	}
	return protocol, pi, err
}

func InterfaceToString(in interface{}) string {
	rs := ""
	switch in.(type) {
	case int:
		rs = strconv.Itoa(in.(int))
	case float64:
		rs = strconv.FormatFloat(in.(float64), 'f', -1, 64)
	case string:
		rs = in.(string)
	case bool:
		rs = strconv.FormatBool(in.(bool))
	}
	return rs
}

// GetLocalIPs ip from ip net
func GetLocalIPs() []string {
	if len(localIPs) == 0 {
		addr, err := net.InterfaceAddrs()
		if err != nil {
			vlog.Warningf("get local ip fail. %s", err.Error())
		} else {
			for _, address := range addr {
				if ipNet, ok := address.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
					if ipNet.IP.To4() != nil {
						localIPs = append(localIPs, ipNet.IP.String())
					}
				}
			}
		}
	}
	return localIPs
}

// GetLocalIP flag of localIP > ip net
func GetLocalIP() string {
	if *LocalIP != "" {
		return *LocalIP
	} else if len(GetLocalIPs()) > 0 {
		return GetLocalIPs()[0]
	}
	return "unknown"
}

func SliceShuffle(slice []string) []string {
	for i := 0; i < len(slice); i++ {
		a := rand.Intn(len(slice))
		b := rand.Intn(len(slice))
		slice[a], slice[b] = slice[b], slice[a]
	}
	return slice
}

func EndpointShuffle(slice []EndPoint) []EndPoint {
	for i := 0; i < len(slice); i++ {
		a := rand.Intn(len(slice))
		b := rand.Intn(len(slice))
		slice[a], slice[b] = slice[b], slice[a]
	}
	return slice
}

func ByteSliceShuffle(slice []byte) []byte {
	for i := 0; i < len(slice); i++ {
		a := rand.Intn(len(slice))
		b := rand.Intn(len(slice))
		slice[a], slice[b] = slice[b], slice[a]
	}
	return slice
}

func FirstUpper(s string) string {
	r := []rune(s)

	if unicode.IsUpper(r[0]) {
		return s
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func GetReqInfo(request Request) string {
	if request != nil {
		var buffer bytes.Buffer
		buffer.WriteString("req{")
		buffer.WriteString(strconv.FormatUint(request.GetRequestID(), 10))
		buffer.WriteString(",")
		buffer.WriteString(request.GetServiceName())
		buffer.WriteString(",")
		buffer.WriteString(request.GetMethod())
		buffer.WriteString("}")
		return buffer.String()
	}
	return ""
}

func GetResInfo(response Response) string {
	if response != nil {
		var buffer bytes.Buffer
		buffer.WriteString("res{")
		buffer.WriteString(strconv.FormatUint(response.GetRequestID(), 10))
		buffer.WriteString(",")
		if response.GetException() != nil {
			buffer.WriteString(response.GetException().ErrMsg)
		}
		buffer.WriteString("}")
		return buffer.String()
	}
	return ""
}

func GetEPFilterInfo(filter EndPointFilter) string {
	if filter != nil {
		var buffer bytes.Buffer
		writeEPFilter(filter, &buffer)
		return buffer.String()
	}
	return ""
}

func writeEPFilter(filter EndPointFilter, buffer *bytes.Buffer) {
	buffer.WriteString(filter.GetName())
	if filter.GetNext() != nil {
		buffer.WriteString("->")
		writeEPFilter(filter.GetNext(), buffer)
	}
}

func HandlePanic(f func()) {
	if err := recover(); err != nil {
		vlog.Errorf("recover panic. error:%v, stack: %s", err, debug.Stack())
		if f != nil {
			f()
		}
		if PanicStatFunc != nil {
			PanicStatFunc()
		}
	}
}

// TrimSplit slices s into all substrings separated by sep and
// returns a slice of the substrings between those separators,
// specially trim all substrings.
func TrimSplit(s string, sep string) []string {
	if sep == "" {
		return strings.Split(s, sep)
	}
	var a []string
	for {
		m := strings.Index(s, sep)
		if m < 0 {
			s = strings.TrimSpace(s)
			break
		}
		temp := strings.TrimSpace(s[:m])
		if temp != "" {
			a = append(a, temp)
		}
		s = s[m+len(sep):]
	}
	if s != "" {
		a = append(a, s)
	}
	return a
}

// TrimSplitSet slices string and convert to map set
func TrimSplitSet(s string, sep string) map[string]bool {
	slice := TrimSplit(s, sep)
	set := make(map[string]bool, len(slice))

	for _, item := range slice {
		set[item] = true
	}

	return set
}

// ListenUnixSock try to listen a unix socket address
// this method using by create motan agent server, management server and http proxy server
func ListenUnixSock(unixSockAddr string) (net.Listener, error) {
	if err := os.RemoveAll(unixSockAddr); err != nil {
		vlog.Errorf("listenUnixSock err, remove old unix sock file fail. err: %v", err)
		return nil, err
	}

	listener, err := net.Listen("unix", unixSockAddr)
	if err != nil {
		vlog.Errorf("listenUnixSock err, listen unix sock fail. err:%v", err)
		return nil, err
	}
	return listener, nil
}

// SlicesUnique deduplicate the values of the slice
func SlicesUnique(src []string) []string {
	var dst []string
	set := map[string]bool{}
	for _, v := range src {
		if !set[v] {
			set[v] = true
			dst = append(dst, v)
		}
	}
	return dst
}

// GetDirectEnvRegistry get the direct registry from the environment variable.
// return registry urls if url match, or return nil
func GetDirectEnvRegistry(url *URL) *URL {
	initDirectEnv.Do(func() {
		str := os.Getenv(DirectRPCEnvironmentName)
		if str != "" {
			vlog.Infof("find env " + DirectRPCEnvironmentName + ":" + str)
			ss := strings.Split(str, ";") // multi services
			for _, s := range ss {
				sInfo := strings.Split(s, ">")
				if len(sInfo) == 2 {
					group := ""
					addr := sInfo[1]
					gIndex := strings.Index(sInfo[1], "@")
					if gIndex > -1 { // has group info
						group = sInfo[1][:gIndex]
						addr = sInfo[1][gIndex+1:]
					}
					reg := &URL{Protocol: "direct", Group: strings.TrimSpace(group)}
					reg.PutParam(AddressKey, strings.TrimSpace(addr))
					key := strings.TrimSpace(sInfo[0])
					vlog.Infof("add direct registry info, key:"+key+", url: %+v", reg)
					if directRpc == nil {
						directRpc = make(map[string]*URL, 16)
					}
					directRpc[key] = reg
				}
			}
		}
	})
	if directRpc != nil { // have direct rpc
		p := url.Path
		for k, u := range directRpc {
			if strings.HasSuffix(k, "*") { // prefix match
				if strings.HasPrefix(p, k[:len(k)-1]) {
					return u
				}
			} else { // suffix match
				if strings.HasSuffix(p, k) {
					return u
				}
			}
		}
	}
	return nil
}

// ClearDirectEnvRegistry is only for unit test
func ClearDirectEnvRegistry() {
	directRpc = nil
	initDirectEnv = sync.Once{}
}

func GetNonNegative(originValue int64) int64 {
	if originValue > 0 {
		return originValue
	}
	return 0x7fffffffffffffff & originValue
}
