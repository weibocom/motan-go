package motan

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"time"

	"github.com/weibocom/motan-go/cluster"
	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
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
	default:
		rw.WriteHeader(s.a.status)
		rw.Write([]byte(http.StatusText(s.a.status)))
	}
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
	}
}

func (i *InfoHandler) getReferService() []byte {
	mbody := body{Service: []rpcService{}}
	i.a.clustermap.Range(func(k, v interface{}) bool {
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
		default:
			Index(rw, req)
		}
	}
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

func (l *LogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	type response struct {
		Code int    `json:"code"`
		Body string `json:"body"`
	}
	enc := json.NewEncoder(w)
	switch r.URL.Path {
	case "/log/level":
		_ = enc.Encode(response{Code: 200, Body: "Level:" + vlog.GetLevel().String()})
	case "/log/set":
		if lvlString := r.FormValue("level"); lvlString != "" {
			var lvl vlog.LogLevel
			if err := lvl.Set(lvlString); err == nil {
				vlog.SetLevel(lvl)
				_ = enc.Encode(response{Code: 200, Body: "set log level:" + lvlString})
				vlog.Infoln("set log level:", lvlString)
			} else {
				_ = enc.Encode(response{Code: 500, Body: "set log level failed. err:" + err.Error()})
				vlog.Warningln("set log level failed. err:", err.Error())
			}
		}
		if access := r.FormValue("access"); access != "" {
			if status, err := strconv.ParseBool(access); err == nil {
				vlog.SetAccessLog(status)
				_ = enc.Encode(response{Code: 200, Body: "set accessLog status:" + access})
				vlog.Infoln("set accessLog status:", status)
			} else {
				_ = enc.Encode(response{Code: 500, Body: "set accessLog status failed. err:" + err.Error()})
				vlog.Warningln("set accessLog status failed. err:", err.Error())
			}
		}
		if metrics := r.FormValue("metrics"); metrics != "" {
			if status, err := strconv.ParseBool(metrics); err == nil {
				vlog.SetMetricsLog(status)
				_ = enc.Encode(response{Code: 200, Body: "set metricsLog status:" + metrics})
				vlog.Infoln("set metricsLog status:", status)
			} else {
				_ = enc.Encode(response{Code: 500, Body: "set metricsLog status failed. err:" + err.Error()})
				vlog.Warningln("set metricsLog status failed. err:", err.Error())
			}
		}
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
	select {
	case <-time.After(d):
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
