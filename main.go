package main

import (
	"flag"
	"fmt"
	"gacache"
	"log"
	"net/http"
)

var db = map[string]string{
	"tom":     "110",
	"resolmi": "20",
	"jerry":   "119",
}

func creatGroup() *gacache.Group {
	return gacache.NewGroup("scores", 2<<10, gacache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[SlowDB] search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
}

//启动缓存服务
func startCacheServer(addr string, addrs []string, gac *gacache.Group) {
	//创建节点Http服务端(实现了Picker接口)
	peers := gacache.NewHTTPPool(addr)
	//将所有节点信息存入
	peers.Set(addrs...)
	//将当前Picker注册进入当前的节点(每个节点只有一个Picker)
	gac.RegisterPeers(peers)
	log.Println("gacache is running at", addr)
	log.Fatal(http.ListenAndServe(addr[7:], peers))
}

//与用户交互的server
func startAPIServer(apiAddr string, gac *gacache.Group) {
	http.Handle("/api", http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			key := r.URL.Query().Get("key")
			view, err := gac.Get(key)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(view.ByteSlice())

		}))
	log.Println("fontend server is running at", apiAddr)
	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))
}

func main() {
	var port int
	var api bool
	//命令行解析
	flag.IntVar(&port, "port", 8001, "Gacache server port")
	flag.BoolVar(&api, "api", false, "Start a api server?")
	flag.Parse()
	//与用户交互的server
	apiAddr := "http://localhost:9999"
	addrMap := map[int]string{
		8001: "http://localhost:8001",
		8002: "http://localhost:8002",
		8003: "http://localhost:8003",
		8004: "http://localhost:8004",
	}
	var addrs []string
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}
	gac := creatGroup()
	if api {
		go startAPIServer(apiAddr, gac) //带api参数的就是本机self
	}
	startCacheServer(addrMap[port], addrs, gac)
}

//http服务端测试
func httpTest() {
	//Http测试
	gacache.NewGroup("scores", 2<<10, gacache.GetterFunc(
		func(key string) ([]byte, error) {
			log.Println("[DB] search key", key)
			if v, ok := db[key]; ok {
				return []byte(v), nil
			}
			return nil, fmt.Errorf("%s not exist", key)
		}))
	addr := "localhost:9999"
	peers := gacache.NewHTTPPool(addr)
	log.Println("gacache is running at", addr)
	log.Fatal(http.ListenAndServe(addr, peers))
}
