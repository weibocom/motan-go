package motan

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/weibocom/motan-go/cluster"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/filter"
	"github.com/weibocom/motan-go/log"
	"github.com/weibocom/motan-go/metrics"
	"github.com/weibocom/motan-go/protocol"
)

// SetAgent : if need agent to do sth, the handler can implement this interface,
// the func SetAgent will called when agent init the handler
type SetAgent interface {
	SetAgent(agent *Agent)
}

// StatusHandler can change http status, such as 200, 503
// the registed services will not available when status is 503, and will available when status change to 200
type StatusHandler struct {
	a *Agent
}

func (s *StatusHandler) SetAgent(agent *Agent) {
	s.a = agent
}

func (s *StatusHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/200":
		s.a.availableAllServices()
		s.a.status = http.StatusOK
		s.a.saveStatus()
		rw.Write([]byte("ok."))
	case "/503":
		s.a.unavailableAllServices()
		s.a.status = http.StatusServiceUnavailable
		s.a.saveStatus()
		rw.Write([]byte("ok."))
	case "/version":
		rw.Write([]byte(Version))
	case "/status":
		rw.Write(s.getStatus())
	case "/registry/status":
		rw.Write(s.getRegistryStatus())
	default:
		rw.WriteHeader(int(s.a.status))
		rw.Write([]byte(http.StatusText(int(s.a.status))))
	}
}

func (s *StatusHandler) getRegistryStatus() []byte {
	type (
		ResultStatus struct {
			Group    string `json:"group"`
			Service  string `json:"service"`
			Registry string `json:"registry"`
			Status   string `json:"status"`
			ErrMsg   string `json:"errMsg"`
			IsCheck  bool   `json:"isCheck"`
		}
		Result struct {
			Status         string         `json:"status"`
			RegistryStatus []ResultStatus `json:"registryStatus"`
		}
	)
	statuses := s.a.GetRegistryStatus()
	var res []ResultStatus
	curAgentStatus := atomic.LoadInt64(&s.a.status)
	var resStatus string
	if curAgentStatus == http.StatusOK {
		resStatus = motan.RegisterSuccess
	} else {
		resStatus = motan.UnregisterSuccess
	}
	for _, j := range statuses {
		for _, k := range j {
			res = append(res, ResultStatus{
				Group:    k.Service.Group,
				Service:  k.Service.Path,
				Registry: k.Service.GetParam(motan.RegistryKey, ""),
				Status:   k.Status,
				ErrMsg:   k.ErrMsg,
				IsCheck:  k.IsCheck,
			})
			if k.IsCheck {
				if curAgentStatus == http.StatusOK {
					if k.Status == motan.RegisterFailed {
						resStatus = k.Status
					} else if k.Status == motan.NotRegister && resStatus != motan.RegisterFailed {
						resStatus = k.Status
					}
				} else {
					if k.Status == motan.UnregisterFailed {
						resStatus = k.Status
					} else if k.Status == motan.NotRegister && resStatus != motan.UnregisterFailed {
						resStatus = k.Status
					}
				}
			}
		}
	}
	resByte, _ := json.Marshal(Result{
		Status:         resStatus,
		RegistryStatus: res,
	})
	return resByte
}

func (s *StatusHandler) getStatus() []byte {
	type (
		MethodStatus struct {
			Name            string `json:"name"`
			PeriodCallCount int64  `json:"period_call_count"`
		}
		ServiceStatus struct {
			Group   string         `json:"group"`
			Name    string         `json:"name"`
			Methods []MethodStatus `json:"methods"`
		}
		Result struct {
			Status                 int             `json:"status"`
			ServicePeriodCallCount int64           `json:"service_period_call_count"`
			Services               []ServiceStatus `json:"services"`
		}
	)
	result := Result{
		Status:   int(s.a.status),
		Services: make([]ServiceStatus, 0, 16),
	}
	s.a.serviceExporters.Range(func(k, v interface{}) bool {
		exporter := v.(motan.Exporter)
		group := exporter.GetURL().Group
		service := exporter.GetURL().Path
		statItem := metrics.GetStatItem(metrics.Escape(group), metrics.Escape(service))
		if statItem == nil {
			return true
		}
		snapshot := statItem.Snapshot()
		if snapshot == nil {
			return true
		}
		serviceInfo := ServiceStatus{
			Group:   group,
			Name:    service,
			Methods: make([]MethodStatus, 0, 16),
		}
		snapshot.RangeKey(func(k string) {
			if !strings.HasSuffix(k, filter.MetricsTotalCountSuffix) {
				return
			}
			method := k[:len(k)-filter.MetricsTotalCountSuffixLen]
			if index := strings.LastIndex(k, ":"); index != -1 {
				method = method[index+1:]
			}
			callCount := snapshot.Count(k)
			result.ServicePeriodCallCount += callCount
			serviceInfo.Methods = append(serviceInfo.Methods, MethodStatus{
				Name:            method,
				PeriodCallCount: callCount,
			})
		})
		result.Services = append(result.Services, serviceInfo)
		return true
	})
	resultBytes, _ := json.MarshalIndent(struct {
		Code int    `json:"code"`
		Body Result `json:"body"`
	}{
		Code: 200,
		Body: result,
	}, "", "    ")
	return resultBytes
}

type InfoHandler struct {
	a *Agent
}

func (i *InfoHandler) SetAgent(agent *Agent) {
	i.a = agent
}

func (i *InfoHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case "/getConfig":
		rw.Write(i.a.getConfigData())
	case "/getReferService":
		rw.Write(i.getReferService())
	case "/getAllService":
		rw.Write(i.getAllServices())
	}
}

func (i *InfoHandler) getAllServices() []byte {
	sMap := make(map[string][]*motan.URL)
	var serviceList []*motan.URL
	for _, s := range i.a.Context.ServiceURLs {
		serviceList = append(serviceList, s)
	}
	sMap["services"] = serviceList
	sMap["dynamic_services"] = i.a.GetDynamicRegistryInfo().RegisterNodes
	b, _ := json.Marshal(sMap)
	return b
}

func (i *InfoHandler) getReferService() []byte {
	mbody := body{Service: []rpcService{}}
	i.a.clusterMap.Range(func(k, v interface{}) bool {
		cls := v.(*cluster.MotanCluster)
		available := cls.IsAvailable()
		mbody.Service = append(mbody.Service, rpcService{Name: k.(string), Status: available})
		return true
	})
	retData := jsonRetData{Code: 200, Body: mbody}
	data, _ := json.Marshal(retData)
	return data
}

type rpcService struct {
	Name   string `json:"name"`
	Status bool   `json:"status"`
}

type body struct {
	Service []rpcService `json:"service"`
}

type jsonRetData struct {
	Code int  `json:"code"`
	Body body `json:"body"`
}

// DebugHandler control pprof dynamically
// ***the func of pprof is copied from net/http/pprof ***
type DebugHandler struct {
	enable bool
}

// ServeHTTP implement handler interface
func (d *DebugHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/debug/pprof/sw" {
		t := req.Header.Get("ctr")
		switch t {
		case "op": // open pprof
			d.enable = true
			rw.Write([]byte("T"))
		case "cp": //close pprof
			d.enable = false
			rw.Write([]byte("F"))
		}
	} else if d.enable {
		switch req.URL.Path {
		case "/debug/pprof/cmdline":
			Cmdline(rw, req)
		case "/debug/pprof/profile":
			Profile(rw, req)
		case "/debug/pprof/symbol":
			Symbol(rw, req)
		case "/debug/pprof/trace":
			Trace(rw, req)
		case "/debug/mesh/trace":
			MeshTrace(rw, req)
		case "/debug/stat/system":
			StatSystem(rw)
		case "/debug/stat/process":
			StatProcess(rw)
		case "/debug/stat/openFiles":
			StatOpenFiles(rw)
		case "/debug/stat/connections":
			StatConnections(rw)
		default:
			Index(rw, req)
		}
	}
}

type StatMemInfo struct {
	MemTotal    uint64  `json:"memTotal(MB)"`
	MemUsed     uint64  `json:"memUsed(MB)"`
	MemPercent  float64 `json:"memPercent"`
	SwapTotal   uint64  `json:"swapTotal(MB)"`
	SwapUsed    uint64  `json:"swapUsed(MB)"`
	SwapPercent float64 `json:"swapPercent"`
}

type StatNetInfo struct {
	Name     string   `json:"name"`
	SentRate uint64   `json:"sentRate(B/s)"`
	RecvRate uint64   `json:"recvRate(B/s)"`
	IP       []string `json:"ip"`
}

type StatConn struct {
	Listen      uint32 `json:"LISTEN"`
	Established uint32 `json:"ESTABLISHED"`
	TimeWait    uint32 `json:"TIME_WAIT"`
}

type StatConnInfo struct {
	Fd         uint32 `json:"fd"`
	Status     string `json:"status"`
	LocalAddr  string `json:"localAddr"`
	RemoteAddr string `json:"remoteAddr"`
}

type StatIOInfo struct {
	ReadTotal  uint64 `json:"readTotal(B)"`
	ReadRate   uint64 `json:"readRate(B/s)"`
	WriteTotal uint64 `json:"writsTotal(B)"`
	WriteRate  uint64 `json:"writsRate(B/s)"`
}

type StatSystemEntity struct {
	CpuCores        int32         `json:"cpuCores"`
	Load1           float64       `json:"load1"`
	Load5           float64       `json:"load5"`
	Load15          float64       `json:"load15"`
	CpuPercent      float64       `json:"cpuPercent"`
	HostName        string        `json:"hostName"`
	Platform        string        `json:"platform"`
	PlatformVersion string        `json:"platformVersion"`
	KernelVersion   string        `json:"kernelVersion"`
	GoVersion       string        `json:"goVersion"`
	BootTime        string        `json:"bootTime"`
	MemInfo         *StatMemInfo  `json:"memInfo"`
	NetInfo         []StatNetInfo `json:"netInfo"`
}

type StatProcessEntity struct {
	NumThreads     int32       `json:"numThreads"`
	NumFDs         int32       `json:"numFds"`
	CpuPercent     float64     `json:"cpuPercent"`
	MemoryPercent  float32     `json:"memoryPercent"`
	UsedMemory     float64     `json:"usedMemory(MB)"`
	OpenFilesCount int32       `json:"openFilesCount"`
	CreateTime     string      `json:"createTime"`
	IO             *StatIOInfo `json:"io"`
	Connections    *StatConn   `json:"connections"`
}

func StatSystem(w http.ResponseWriter) {
	var cpuCores int32
	c, _ := cpu.Info()
	for _, value := range c {
		cpuCores += value.Cores
	}
	virtual, _ := mem.VirtualMemory()
	swap, _ := mem.SwapMemory()
	memInfo := StatMemInfo{
		MemTotal:    virtual.Total >> 20,
		MemUsed:     virtual.Used >> 20,
		MemPercent:  oneDecimal(virtual.UsedPercent),
		SwapTotal:   swap.Total >> 20,
		SwapUsed:    swap.Used >> 20,
		SwapPercent: oneDecimal(swap.UsedPercent),
	}
	netInfoPre := make(map[string]net.IOCountersStat)
	netIOPre, _ := net.IOCounters(true)
	for _, n := range netIOPre {
		netInfoPre[n.Name] = n
	}
	if netIoAllPre, err := net.IOCounters(false); err == nil {
		netInfoPre[netIoAllPre[0].Name] = netIoAllPre[0]
	}
	time.Sleep(time.Second)
	interfaceList, _ := net.Interfaces()
	interfaceMap := make(map[string][]string)
	for _, inter := range interfaceList {
		var addrList []string
		for _, addr := range inter.Addrs {
			addrList = append(addrList, addr.Addr)
		}
		interfaceMap[inter.Name] = addrList
	}
	netIOEnd, _ := net.IOCounters(true)
	var netInfo []StatNetInfo
	for _, n := range netIOEnd {
		netInfo = append(netInfo, StatNetInfo{
			Name:     n.Name,
			IP:       interfaceMap[n.Name],
			SentRate: n.BytesSent - netInfoPre[n.Name].BytesSent,
			RecvRate: n.BytesRecv - netInfoPre[n.Name].BytesRecv,
		})
	}
	if netIoAllPre, err := net.IOCounters(false); err == nil {
		netInfo = append(netInfo, StatNetInfo{
			Name:     netIoAllPre[0].Name,
			SentRate: netIoAllPre[0].BytesSent - netInfoPre[netIoAllPre[0].Name].BytesSent,
			RecvRate: netIoAllPre[0].BytesRecv - netInfoPre[netIoAllPre[0].Name].BytesRecv,
		})
	}
	cpuPercent, _ := cpu.Percent(time.Second, false)
	var cpuPer float64
	if len(cpuPercent) > 0 {
		cpuPer = cpuPercent[0]
	}
	l, _ := load.Avg()
	n, _ := host.Info()
	hTime, _ := host.BootTime()
	statSystem := StatSystemEntity{
		HostName:        n.Hostname,
		Platform:        n.Platform,
		PlatformVersion: n.PlatformVersion,
		KernelVersion:   n.KernelVersion,
		GoVersion:       runtime.Version(),
		BootTime:        time.Unix(int64(hTime), 0).String(),
		CpuCores:        cpuCores,
		MemInfo:         &memInfo,
		NetInfo:         netInfo,
		Load1:           l.Load1,
		Load5:           l.Load5,
		Load15:          l.Load15,
		CpuPercent:      oneDecimal(cpuPer),
	}
	data, _ := json.Marshal(statSystem)
	_, _ = w.Write(data)
}

func StatProcess(w http.ResponseWriter) {
	checkPid := os.Getpid()
	p, _ := process.NewProcess(int32(checkPid))
	numThreads, _ := p.NumThreads()
	numFDs, _ := p.NumFDs()
	cpuPercent, _ := p.CPUPercent()
	connections, _ := p.Connections()
	connInfo := StatConn{}
	for _, value := range connections {
		switch value.Status {
		case "LISTEN":
			connInfo.Listen++
		case "ESTABLISHED":
			connInfo.Established++
		case "TIME_WAIT":
			connInfo.TimeWait++
		}
	}
	createTime, _ := p.CreateTime()
	memInfo, _ := p.MemoryInfo()
	memoryPercent, _ := p.MemoryPercent()
	openFiles, _ := p.OpenFiles()
	var ioCounters *StatIOInfo
	if ioCountersStart, err := p.IOCounters(); err == nil {
		time.Sleep(time.Second)
		ioCountersEnd, _ := p.IOCounters()
		ioCounters = &StatIOInfo{
			ReadTotal:  ioCountersEnd.ReadBytes,
			ReadRate:   ioCountersEnd.ReadBytes - ioCountersStart.ReadBytes,
			WriteTotal: ioCountersEnd.WriteBytes,
			WriteRate:  ioCountersEnd.WriteBytes - ioCountersStart.WriteBytes,
		}
	}
	statProcess := StatProcessEntity{
		NumThreads:     numThreads,
		NumFDs:         numFDs,
		CpuPercent:     oneDecimal(cpuPercent),
		CreateTime:     time.Unix(createTime/1000, 0).String(),
		MemoryPercent:  float32(oneDecimal(float64(memoryPercent))),
		UsedMemory:     oneDecimal(float64(memInfo.RSS) / 1000000),
		IO:             ioCounters,
		OpenFilesCount: int32(len(openFiles)),
		Connections:    &connInfo,
	}
	data, _ := json.Marshal(statProcess)
	_, _ = w.Write(data)
}

func oneDecimal(value float64) float64 {
	return math.Trunc(value*10+0.5) / 10
}

func StatOpenFiles(w http.ResponseWriter) {
	checkPid := os.Getpid()
	p, _ := process.NewProcess(int32(checkPid))
	openFiles, _ := p.OpenFiles()
	data, _ := json.Marshal(openFiles)
	_, _ = w.Write(data)
}

func StatConnections(w http.ResponseWriter) {
	checkPid := os.Getpid()
	p, _ := process.NewProcess(int32(checkPid))
	connections, _ := p.Connections()
	var conns []StatConnInfo
	for _, value := range connections {
		conns = append(conns, StatConnInfo{
			Fd:         value.Fd,
			Status:     value.Status,
			LocalAddr:  value.Laddr.IP + ":" + strconv.FormatUint(uint64(value.Laddr.Port), 10),
			RemoteAddr: value.Raddr.IP + ":" + strconv.FormatUint(uint64(value.Raddr.Port), 10),
		})
	}
	data, _ := json.Marshal(conns)
	_, _ = w.Write(data)
}

func MeshTrace(w http.ResponseWriter, r *http.Request) {
	sec, _ := strconv.ParseInt(r.FormValue("seconds"), 10, 64)
	if sec == 0 {
		sec = 30
	}

	addr := strings.TrimSpace(r.FormValue("addr"))
	group := strings.TrimSpace(r.FormValue("group"))
	path := strings.TrimSpace(r.FormValue("service"))
	ratio, _ := strconv.ParseInt(r.FormValue("ratio"), 10, 64) // percentage 1-100
	ct := &CustomTrace{addr: addr, group: group, path: path, ratio: int(ratio)}
	oldTrace := motan.TracePolicy
	motan.TracePolicy = ct.Trace
	sleep(w, time.Duration(sec)*time.Second)
	motan.TracePolicy = oldTrace
	tcs := motan.GetTraceContexts()
	fmt.Fprintf(w, "mesh trace finish. trace size:%d， time unit:ns\n", len(tcs))
	for i, tc := range tcs {
		fmt.Fprintf(w, "{\"No\":%d,\"trace\":%s}\n", i, formatTc(tc))
	}
}

func formatTc(tc *motan.TraceContext) string {
	processReqSpan(tc.ReqSpans)
	processResSpan(tc.ResSpans)
	if len(tc.ReqSpans) > 0 && len(tc.ResSpans) > 0 {
		tc.Values["requestTime"] = strconv.FormatInt(tc.ReqSpans[len(tc.ReqSpans)-1].Time.UnixNano()-tc.ReqSpans[0].Time.UnixNano(), 10)
		tc.Values["responseTime"] = strconv.FormatInt(tc.ResSpans[len(tc.ResSpans)-1].Time.UnixNano()-tc.ResSpans[0].Time.UnixNano(), 10)
		tc.Values["remoteTime"] = strconv.FormatInt(tc.ResSpans[0].Time.UnixNano()-tc.ReqSpans[len(tc.ReqSpans)-1].Time.UnixNano(), 10)
		tc.Values["totalTime"] = strconv.FormatInt(tc.ResSpans[len(tc.ResSpans)-1].Time.UnixNano()-tc.ReqSpans[0].Time.UnixNano(), 10)
	}
	data, _ := json.MarshalIndent(tc, "", "    ")
	return string(data)
}

func processReqSpan(spans []*motan.Span) {
	m := make(map[string]int64, 16)
	var defaultLastTime int64
	for _, rqs := range spans {
		if rqs.Addr == "" {
			if defaultLastTime > 0 {
				rqs.Duration = rqs.Time.UnixNano() - defaultLastTime
			} else {
				rqs.Duration = 0
			}
			defaultLastTime = rqs.Time.UnixNano()
		} else {
			if t, ok := m[rqs.Addr]; ok {
				rqs.Duration = rqs.Time.UnixNano() - t
			} else if defaultLastTime > 0 {
				rqs.Duration = rqs.Time.UnixNano() - defaultLastTime
			} else {
				rqs.Duration = 0
			}
			m[rqs.Addr] = rqs.Time.UnixNano()
		}
	}
}

func processResSpan(spans []*motan.Span) {
	var lastTime int64
	for _, rqs := range spans {
		if lastTime > 0 {
			rqs.Duration = rqs.Time.UnixNano() - lastTime
		} else {
			rqs.Duration = 0
		}
		lastTime = rqs.Time.UnixNano()
	}
}

type CustomTrace struct {
	path  string
	group string
	addr  string
	ratio int
}

func (c *CustomTrace) Trace(rid uint64, ext *motan.StringMap) *motan.TraceContext {
	if c.addr != "" {
		addr := ext.LoadOrEmpty(motan.HostKey)
		if !strings.HasPrefix(addr, c.addr) {
			return nil
		}
	}
	if c.group != "" {
		group := ext.LoadOrEmpty(protocol.MGroup)
		if group != c.group {
			return nil
		}
	}
	if c.path != "" {
		path := ext.LoadOrEmpty(protocol.MPath)
		if path != c.path {
			return nil
		}
	}
	if c.ratio > 0 && c.ratio < 100 {
		n := rand.Intn(100)
		if n >= c.ratio {
			return nil
		}
	}
	return motan.NewTraceContext(rid)
}

type SwitcherHandler struct{}

func (s *SwitcherHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	value := r.FormValue("value")
	switcher := motan.GetSwitcherManager()
	switch r.URL.Path {
	case "/switcher/set":
		if name == "" {
			fmt.Fprintf(w, "Please specify a switcher name!")
			return
		}
		if value == "" {
			fmt.Fprintf(w, "Please specify a switcher value!")
			return
		}
		valueBool, err := strconv.ParseBool(value)
		if err != nil {
			fmt.Fprintf(w, "Invalid switcher value(must be Bool): %s", value)
			return
		}
		s := switcher.GetSwitcher(name)
		if s == nil {
			fmt.Fprintf(w, "Not a registered switcher, name: %s", name)
			return
		}
		s.SetValue(valueBool)
		fmt.Fprintf(w, "Set switcher %s value to %s !", name, value)
	case "/switcher/get":
		if name == "" {
			fmt.Fprintf(w, "Please specify a switcher name!")
			return
		}
		s := switcher.GetSwitcher(name)
		if s == nil {
			fmt.Fprintf(w, "Not a registered switcher, name: %s", name)
			return
		}
		value := s.IsOpen()
		fmt.Fprintf(w, "Switcher value for %s is %v!", name, value)
	case "/switcher/getAll":
		result := switcher.GetAllSwitchers()
		b, _ := json.Marshal(result)
		w.Write(b)
	}
}

type LogHandler struct{}

type logResponse struct {
	Code int    `json:"code"`
	Body string `json:"body"`
}

func (l *LogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	jsonEncoder := json.NewEncoder(w)
	type resBody struct {
		Level      string `json:"level"`
		AccessLog  bool   `json:"accessLog"`
		MetricsLog bool   `json:"metricsLog"`
	}
	switch r.URL.Path {
	case "/logConfig/get":
		body, _ := json.Marshal(resBody{
			Level:      vlog.GetLevel().String(),
			AccessLog:  vlog.GetAccessLogAvailable(),
			MetricsLog: vlog.GetMetricsLogAvailable()})
		_ = jsonEncoder.Encode(logResponse{
			Code: 200,
			Body: string(body)})
	case "/logConfig/set":
		if lvlString := r.FormValue("level"); lvlString != "" {
			var lvl vlog.LogLevel
			if err := lvl.Set(lvlString); err == nil {
				vlog.SetLevel(lvl)
				_ = jsonEncoder.Encode(logResponse{Code: 200, Body: "set log level:" + lvlString})
				vlog.Infoln("set log level:", lvlString)
			} else {
				_ = jsonEncoder.Encode(logResponse{Code: 500, Body: "set log level failed. err:" + err.Error()})
				vlog.Warningln("set log level failed. err:", err.Error())
			}
		} else if available := r.FormValue("access"); available != "" {
			setLogStatus(jsonEncoder, "accessLog", available)
		} else if available := r.FormValue("metrics"); available != "" {
			setLogStatus(jsonEncoder, "metricsLog", available)
		}
	}
}

func setLogStatus(jsonEncoder *json.Encoder, logType, available string) {
	if status, err := strconv.ParseBool(available); err == nil {
		switch logType {
		case "accessLog":
			vlog.SetAccessLogAvailable(status)
		case "metricsLog":
			vlog.SetMetricsLogAvailable(status)
		}
		_ = jsonEncoder.Encode(logResponse{Code: 200, Body: "set " + logType + " status:" + available})
		vlog.Infoln("set "+logType+" status:", status)
	} else {
		_ = jsonEncoder.Encode(logResponse{Code: 500, Body: "set " + logType + " status failed. err:" + err.Error()})
		vlog.Warningln("set "+logType+" status failed. err:", err.Error())
	}
}

type HotReload struct {
	agent *Agent
}

func (h *HotReload) SetAgent(agent *Agent) {
	h.agent = agent
}

func (h *HotReload) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/reload/clusters":
		ctx := &motan.Context{ConfigFile: h.agent.ConfigFile}
		ctx.Initialize()

		h.agent.reloadClusters(ctx)

		refersURLs, _ := json.Marshal(ctx.RefersURLs)

		jsonEncoder := json.NewEncoder(w)
		_ = jsonEncoder.Encode(logResponse{
			Code: 200,
			Body: string(refersURLs),
		})
	}
}

//------------ below code is copied from net/http/pprof -------------

// Cmdline responds with the running program's
// command line, with arguments separated by NUL bytes.
// The package initialization registers it as /debug/pprof/cmdline.
func Cmdline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, strings.Join(os.Args, "\x00"))
}

func sleep(w http.ResponseWriter, d time.Duration) {
	var clientGone <-chan bool
	if cn, ok := w.(http.CloseNotifier); ok {
		clientGone = cn.CloseNotify()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-clientGone:
	}
}

// Profile responds with the pprof-formatted cpu profile.
// The package initialization registers it as /debug/pprof/profile.
func Profile(w http.ResponseWriter, r *http.Request) {
	sec, _ := strconv.ParseInt(r.FormValue("seconds"), 10, 64)
	if sec == 0 {
		sec = 30
	}

	// Set Content Type assuming StartCPUProfile will work,
	// because if it does it starts writing.
	w.Header().Set("Content-Type", "application/octet-stream")
	if err := pprof.StartCPUProfile(w); err != nil {
		// StartCPUProfile failed, so no writes yet.
		// Can change header back to text content
		// and send error code.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not enable CPU profiling: %s\n", err)
		return
	}
	sleep(w, time.Duration(sec)*time.Second)
	pprof.StopCPUProfile()
}

// Trace responds with the execution trace in binary form.
// Tracing lasts for duration specified in seconds GET parameter, or for 1 second if not specified.
// The package initialization registers it as /debug/pprof/trace.
func Trace(w http.ResponseWriter, r *http.Request) {
	sec, err := strconv.ParseFloat(r.FormValue("seconds"), 64)
	if sec <= 0 || err != nil {
		sec = 1
	}

	// Set Content Type assuming trace.Start will work,
	// because if it does it starts writing.
	w.Header().Set("Content-Type", "application/octet-stream")
	if err := trace.Start(w); err != nil {
		// trace.Start failed, so no writes yet.
		// Can change header back to text content and send error code.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Could not enable tracing: %s\n", err)
		return
	}
	sleep(w, time.Duration(sec*float64(time.Second)))
	trace.Stop()
}

// Symbol looks up the program counters listed in the request,
// responding with a table mapping program counters to function names.
// The package initialization registers it as /debug/pprof/symbol.
func Symbol(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	// We have to read the whole POST body before
	// writing any output. Buffer the output here.
	var buf bytes.Buffer

	// We don't know how many symbols we have, but we
	// do have symbol information. Pprof only cares whether
	// this number is 0 (no symbols available) or > 0.
	fmt.Fprintf(&buf, "num_symbols: 1\n")

	var b *bufio.Reader
	if r.Method == "POST" {
		b = bufio.NewReader(r.Body)
	} else {
		b = bufio.NewReader(strings.NewReader(r.URL.RawQuery))
	}

	for {
		word, err := b.ReadSlice('+')
		if err == nil {
			word = word[0 : len(word)-1] // trim +
		}
		pc, _ := strconv.ParseUint(string(word), 0, 64)
		if pc != 0 {
			f := runtime.FuncForPC(uintptr(pc))
			if f != nil {
				fmt.Fprintf(&buf, "%#x %s\n", pc, f.Name())
			}
		}

		// Wait until here to check for err; the last
		// symbol will have an err because it doesn't end in +.
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(&buf, "reading request: %v\n", err)
			}
			break
		}
	}

	w.Write(buf.Bytes())
}

// Handler returns an HTTP handler that serves the named profile.
func Handler(name string) http.Handler {
	return handler(name)
}

type handler string

func (name handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	debug, _ := strconv.Atoi(r.FormValue("debug"))
	p := pprof.Lookup(string(name))
	if p == nil {
		w.WriteHeader(404)
		fmt.Fprintf(w, "Unknown profile: %s\n", name)
		return
	}
	gc, _ := strconv.Atoi(r.FormValue("gc"))
	if name == "heap" && gc > 0 {
		runtime.GC()
	}
	p.WriteTo(w, debug)
	return
}

// Index responds with the pprof-formatted profile named by the request.
// For example, "/debug/pprof/heap" serves the "heap" profile.
// Index responds to a request for "/debug/pprof/" with an HTML page
// listing the available profiles.
func Index(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/debug/pprof/") {
		name := strings.TrimPrefix(r.URL.Path, "/debug/pprof/")
		if name != "" {
			handler(name).ServeHTTP(w, r)
			return
		}
	}

	profiles := pprof.Profiles()
	if err := indexTmpl.Execute(w, profiles); err != nil {
		log.Print(err)
	}
}

var indexTmpl = template.Must(template.New("index").Parse(`<html>
<head>
<title>/debug/pprof/</title>
</head>
<body>
/debug/pprof/<br>
<br>
profiles:<br>
<table>
{{range .}}
<tr><td align=right>{{.Count}}<td><a href="{{.Name}}?debug=1">{{.Name}}</a>
{{end}}
</table>
<br>
<a href="goroutine?debug=2">full goroutine stack dump</a><br>
</body>
</html>
`))
