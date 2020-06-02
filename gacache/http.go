package gacache

import (
	"fmt"
	"gacache/consistenthash"
	pb "gacache/gacachepb"
	"github.com/golang/protobuf/proto"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	defaultPath     = "/_gacache/"
	defaultReplicas = 50
)

type HTTPPool struct {
	self        string //自己的地址,包括ip:port
	basePath    string //节点间通讯地址的前缀,默认是'/_gacache/'
	mu          sync.Mutex
	peers       *consistenthash.Map    //一致性Hash算法
	httpGetters map[string]*httpGetter //每个远程节点对应一个httpGetter(节点的ip:port/defaultPath)
}

func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basePath: defaultPath,
	}
}

func (p *HTTPPool) Log(format string, v ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, v...))
}

//http服务端
func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !strings.HasPrefix(req.URL.Path, p.basePath) {
		panic("HTTPPool serving unexpect path")
	}
	p.Log("%s %s", req.Method, req.URL.Path)
	// basePath/groupName/key
	// 以‘/’为界限将groupName和key划分为2个part
	parts := strings.SplitN(req.URL.Path[len(p.basePath):], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	groupName := parts[0]
	key := parts[1]
	group := GetGroup(groupName)
	if group == nil {
		http.Error(w, "no such group: "+groupName, http.StatusNotFound)
		return
	}
	view, err := group.Get(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	//使用proto编码Http响应
	body, err := proto.Marshal(&pb.Response{Value: view.ByteSlice()})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(body)
}

//设置多节点
func (p *HTTPPool) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers = consistenthash.New(defaultReplicas, nil)
	p.peers.Add(peers...)
	p.httpGetters = make(map[string]*httpGetter, len(peers))
	for _, peer := range peers {
		//peer就是节点地址
		p.httpGetters[peer] = &httpGetter{baseURL: peer + p.basePath}
	}
}

//利用一致性Hash选择节点
func (p *HTTPPool) PickPeer(key string) (PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if peer := p.peers.Get(key); peer != "" && peer != p.self {
		p.Log("Pick peer %s", peer)
		return p.httpGetters[peer], true
	}
	return nil, false
}

//检查接口
var _ PeerPicker = (*HTTPPool)(nil)

//http客户端,用于向远程节点请求数据
//其实可以直接理解为存远程节点的地址的结构 eg. localhost:8002/defaultPath
type httpGetter struct {
	baseURL string
}

//通过节点地址和groupName以及key构成的地址请求数据,通过proto解码数据
func (h *httpGetter) Get(in *pb.Request, out *pb.Response) error {
	u := fmt.Sprintf(
		"%v%v/%v",
		h.baseURL,
		url.QueryEscape(in.GetGroup()),
		url.QueryEscape(in.GetKey()),
	)
	//通过http请求远程节点的数据
	res, err := http.Get(u)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned: %v", res.Status)
	}
	//转换成[]byte
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %v", err)
	}
	//解码proto并将结果存到out中
	if err != proto.Unmarshal(bytes, out) {
		return fmt.Errorf("decoding response body : %v", err)
	}
	return nil
}

//接口实现判断
var _ PeerGetter = (*httpGetter)(nil)
