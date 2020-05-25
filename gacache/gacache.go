package gacache

import (
	"fmt"
	pb "gacache/gacachepb"
	"gacache/singleflight"
	"log"
	"sync"
)

type Getter interface {
	Get(key string) ([]byte, error)
}

type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

type Group struct {
	name      string
	getter    Getter
	mainCache cache
	peers     PeerPicker
	//singleflight并发请求控制
	loader *singleflight.Group
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

//新建Group
func NewGroup(name string, cacheByte int64, getter Getter) *Group {
	if getter == nil {
		panic("nil Getter")
	}
	mu.Lock()
	defer mu.Unlock()
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cacheBytes: cacheByte},
		loader:    &singleflight.Group{},
	}
	groups[name] = g
	return g
}

//获取Group
func GetGroup(name string) *Group {
	mu.RLock() //只读操作，用读锁就ok了
	defer mu.RUnlock()
	g := groups[name]
	return g
}

func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, fmt.Errorf("key nil")
	}
	if v, ok := g.mainCache.get(key); ok {
		log.Printf("[GaCache] hit")
		return v, nil
	}
	//当前节点没有数据,去其他地方加载
	return g.load(key)
}

func (g *Group) load(key string) (value ByteView, err error) {
	//通过singleflight去加载
	view, err := g.loader.Do(key, func() (interface{}, error) {
		if g.peers != nil {
			//根据一致性Hash选择节点Peer
			if peer, ok := g.peers.PickPeer(key); ok {
				//从上面的Peer中获取数据
				if value, err = g.getFromPeer(peer, key); err == nil {
					return value, nil
				}
				log.Println("[Gacache] Fail to get from remote peer!!!", err)
			}
		}
		return g.getLocally(key)
	})
	if err == nil {
		return view.(ByteView), nil
	}
	return
}

//从远程节点获取数据
func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	//构建proto的message
	req := &pb.Request{
		Group: g.name,
		Key:   key,
	}
	res := &pb.Response{}
	err := peer.Get(req, res)

	fmt.Println("getFromPeer", key)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: res.Value}, nil
}

//从数据源获取数据
func (g *Group) getLocally(key string) (ByteView, error) {
	bytes, err := g.getter.Get(key) //回调函数，从数据源取数据
	if err != nil {
		return ByteView{}, err
	}
	//将数据源的数据拷贝一份放入cache中，防止其他外部程序占有该数据并修改
	value := ByteView{b: cloneBytes(bytes)}
	g.populateCache(key, value)
	return value, nil
}

//将从数据源获取的数据加入cache
func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.put(key, value)
}

func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeers called more than once ! ! !")
	}
	g.peers = peers
}
