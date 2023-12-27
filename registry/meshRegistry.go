package registry

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	motan "github.com/weibocom/motan-go/core"
	"github.com/weibocom/motan-go/log"
)

const defaultMeshRegistryHost = "localhost"
const defaultMeshRegistryPort = 8002
const meshRegistryRequestContentType = "application/json;charset=utf-8"

type dynamicConfigResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Body    interface{} `json:"body"`
}

type MeshRegistry struct {
	url               *motan.URL
	meshPort          int
	proxyRegistry     string
	registeredService map[string]*motan.URL
	subscribedService map[string]*motan.URL
	registerLock      sync.Mutex
	subscribeLock     sync.Mutex
}

func (r *MeshRegistry) GetRuntimeInfo() map[string]interface{} {
	return map[string]interface{}{
		motan.RuntimeNameKey: r.GetName(),
	}
}

func (r *MeshRegistry) Initialize() {
	r.registeredService = make(map[string]*motan.URL)
	r.subscribedService = make(map[string]*motan.URL)
	if r.url.Host == "" {
		r.url.Host = defaultMeshRegistryHost
	}
	if r.url.Port == 0 {
		r.url.Port = defaultMeshRegistryPort
	}
	r.proxyRegistry = r.url.GetParam(motan.ProxyRegistryKey, "")
	if r.proxyRegistry == "" {
		panic("Mesh registry should specify the proxyRegistry")
	}

	var info struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Body    struct {
			MeshPort int `json:"mesh_port"`
		} `json:"body"`
	}

	for i := 0; i < 3; i++ {
		resp, err := http.Get("http://" + r.url.Host + ":" + r.url.GetPortStr() + "/registry/info")
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		r.readMeshRegistryResponseStruct(resp, &info)
		r.meshPort = info.Body.MeshPort
	}
}

func (r *MeshRegistry) GetURL() *motan.URL {
	return r.url
}

func (r *MeshRegistry) SetURL(url *motan.URL) {
	r.url = url
}

func (r *MeshRegistry) GetName() string {
	return "mesh"
}

func (r *MeshRegistry) Subscribe(url *motan.URL, listener motan.NotifyListener) {
	vlog.Infof("Subscribe url [%s] to mesh registry [%s]", url.GetIdentity(), r.url.GetIdentity())
	r.subscribeLock.Lock()
	defer r.subscribeLock.Unlock()
	if _, ok := r.subscribedService[url.GetIdentity()]; ok {
		return
	}
	reqData, _ := r.initRegistryRequest(url)
	for {
		// TODO: what to do when failed
		resp, err := http.Post("http://"+r.url.Host+":"+r.url.GetPortStr()+"/registry/subscribe",
			meshRegistryRequestContentType,
			bytes.NewReader(reqData))
		if err != nil {
			vlog.Errorf("Subscribe url %s request registry failed: %s", url.GetIdentity(), err.Error())
			time.Sleep(1 * time.Second)
			continue
		}
		response, err := r.readMeshRegistryResponse(resp)
		if err != nil {
			vlog.Errorf("Subscribe url %s read response failed: %s", url.GetIdentity(), err.Error())
			time.Sleep(1 * time.Second)
			continue
		}
		if response.Code == 200 {
			r.subscribedService[url.GetIdentity()] = url
		} else {
			vlog.Errorf("Subscribe url %s failed: %s", url.GetIdentity(), response.Message)
		}
		return
	}
}

func (r *MeshRegistry) Unsubscribe(url *motan.URL, listener motan.NotifyListener) {
}

func (r *MeshRegistry) Discover(url *motan.URL) []*motan.URL {
	newURL := url.Copy()
	newURL.Host = r.url.Host
	newURL.Port = r.meshPort
	return []*motan.URL{newURL}
}

func (r *MeshRegistry) Register(url *motan.URL) {
	vlog.Infof("Register url [%s] to mesh registry [%s]", url.GetIdentity(), r.url.GetIdentity())
	r.registerLock.Lock()
	defer r.registerLock.Unlock()
	if _, ok := r.registeredService[url.GetIdentity()]; ok {
		return
	}
	reqData, _ := r.initRegistryRequest(url)
	for {
		resp, err := http.Post("http://"+r.url.Host+":"+r.url.GetPortStr()+"/registry/register",
			meshRegistryRequestContentType,
			bytes.NewReader(reqData))
		if err != nil {
			vlog.Errorf("Register url %s request registry failed: %s", url.GetIdentity(), err.Error())
			time.Sleep(1 * time.Second)
			continue
		}
		response, err := r.readMeshRegistryResponse(resp)
		if err != nil {
			vlog.Errorf("Register url %s read response failed: %s", url.GetIdentity(), err.Error())
			time.Sleep(1 * time.Second)
			continue
		}
		if response.Code == 200 {
			r.registeredService[url.GetIdentity()] = url
		} else {
			vlog.Errorf("Register url %s failed: %s", url.GetIdentity(), response.Message)
		}
		return
	}
}

func (r *MeshRegistry) UnRegister(url *motan.URL) {
	vlog.Infof("UnRegister url [%s] from mesh registry [%s]", url.GetIdentity(), r.url.GetIdentity())
	r.registerLock.Lock()
	defer r.registerLock.Unlock()
	if _, ok := r.registeredService[url.GetIdentity()]; !ok {
		return
	}
	reqData, _ := r.initRegistryRequest(url)
	resp, err := http.Post("http://"+r.url.Host+":"+r.url.GetPortStr()+"/registry/unregister",
		meshRegistryRequestContentType,
		bytes.NewReader(reqData))
	if err != nil {
		vlog.Errorf("Unregister url %s request registry failed: %s", url.GetIdentity(), err.Error())
		return
	}
	response, err := r.readMeshRegistryResponse(resp)
	if err != nil {
		vlog.Errorf("Unregister url %s read response failed: %s", url.GetIdentity(), err.Error())
		return
	}
	if response.Code == 200 {
		delete(r.registeredService, url.GetIdentity())
	} else {
		vlog.Errorf("Unregister url %s failed: %s", url.GetIdentity(), response.Message)
	}
}

func (r *MeshRegistry) initRegistryRequest(url *motan.URL) ([]byte, error) {
	url = url.Copy()
	url.PutParam(motan.ProxyRegistryKey, r.proxyRegistry)
	return json.Marshal(url)
}

func (r *MeshRegistry) readMeshRegistryResponse(resp *http.Response) (*dynamicConfigResponse, error) {
	response := new(dynamicConfigResponse)
	err := r.readMeshRegistryResponseStruct(resp, response)
	return response, err
}

func (r *MeshRegistry) readMeshRegistryResponseStruct(resp *http.Response, v interface{}) error {
	body := resp.Body
	defer body.Close()
	resData, err := ioutil.ReadAll(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(resData, v)
}

func (r *MeshRegistry) Available(serverURL *motan.URL) {
}

func (r *MeshRegistry) Unavailable(serverURL *motan.URL) {
}

func (r *MeshRegistry) GetRegisteredServices() []*motan.URL {
	r.registerLock.Lock()
	defer r.registerLock.Unlock()
	urls := make([]*motan.URL, 0, len(r.registeredService))
	for _, u := range r.registeredService {
		urls = append(urls, u)
	}
	return urls
}

func (r *MeshRegistry) StartSnapshot(conf *motan.SnapshotConf) {}
