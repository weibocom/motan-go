package core

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"unicode"

	"github.com/weibocom/motan-go/log"
)

var localIps []string = make([]string, 0)

const (
	defaultServerPort = "9982"
	defaultProtocal   = "motan2"
)

func GetCtxKey(group, version, protocol, path string) string {
	return fmt.Sprintf("%s_%s_%s_%s_", group, version, protocol, path)
}

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

// ip from ipnet
func GetLocalIps() []string {
	if len(localIps) == 0 {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			vlog.Warningf("get local ip fail. %s", err.Error())
		} else {
			for _, address := range addrs {
				if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						localIps = append(localIps, ipnet.IP.String())
					}
				}
			}
		}
	}
	return localIps
}

// falg of localIp > ipnet
func GetLocalIp() string {
	if *LocalIp != "" {
		return *LocalIp
	} else if len(GetLocalIps()) > 0 {
		return GetLocalIps()[0]
	}
	return "unknown"
}

func Slice_shuffle(slice []string) []string {
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
	} else {
		r[0] = unicode.ToUpper(r[0])
		return string(r)
	}
}

func GetReqInfo(request Request) string {
	var buffer bytes.Buffer
	buffer.WriteString("req{")
	buffer.WriteString(strconv.FormatUint(request.GetRequestId(), 10))
	buffer.WriteString(",")
	buffer.WriteString(request.GetServiceName())
	buffer.WriteString(",")
	buffer.WriteString(request.GetMethod())
	buffer.WriteString("}")
	return buffer.String()
}
