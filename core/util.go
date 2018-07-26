package core

import (
	"bytes"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"unicode"

	"github.com/weibocom/motan-go/log"
	"runtime/debug"
)

var localIPs = make([]string, 0)

const (
	defaultServerPort = "9982"
	defaultProtocal   = "motan2"
)

func ParseExportInfo(export string) (string, int, error) {
	port := defaultServerPort
	protocol := defaultProtocal
	s := strings.Split(export, ":")
	if len(s) == 1 && s[0] != "" {
		port = s[0]
	} else if len(s) == 2 {
		if s[0] != "" {
			protocol = s[0]
		}
		port = s[1]
	}
	porti, err := strconv.Atoi(port)
	if err != nil {
		vlog.Errorf("export port not int. port:%s\n", port)
		return protocol, porti, err
	}
	return protocol, porti, err
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

// GetLocalIPs ip from ipnet
func GetLocalIPs() []string {
	if len(localIPs) == 0 {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			vlog.Warningf("get local ip fail. %s", err.Error())
		} else {
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						localIPs = append(localIPs, ipnet.IP.String())
					}
				}
			}
		}
	}
	return localIPs
}

// GetLocalIP falg of localIP > ipnet
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

func HandlePanic(f func()) {
	if err := recover(); err != nil {
		vlog.Errorf("recover panic. error:%v, stack: %s\n", err, debug.Stack())
		if f != nil {
			f()
		}
	}
}

// Split slices s into all substrings separated by sep and
// returns a slice of the substrings between those separators,
// specially trim all substrings and exclude empty substrings.
func TrimSplit(s string, sep string) []string {
	n := strings.Count(s, sep) + 1
	a := make([]string, n)
	i := 0
	for {
		m := strings.Index(s, sep)
		if m < 0 {
			s = strings.TrimSpace(s)
			break
		}
		if temp := strings.TrimSpace(s[:m]); len(temp) > 0 {
			a[i] = temp
			i++
		}
		s = s[m+len(sep):]
	}
	if len(s) > 0 {
		a[i] = s
		return a[:i+1]
	}
	return a[:i]
}
