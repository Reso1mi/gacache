package gacache

import (
	"fmt"
	pb "gacache/gacachepb"
	"gacache/singleflight"
	"log"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

//每分钟远程获取的QPS上限
const maxMinuteRemoteQPS = 10

//cache miss时候的回调接口
type Getter interface {
	Get(key string) ([]byte, error)
}

//cache miss时候的回调函数，实现了上面的回调接口，方便调用者将匿名函数转换为接口传参
//Q: 为什么不直接传递函数？而要传递一个接口？
//A: 传递接口更加的通用，方便扩展，比如在接口内新增加方法，如果传递函数就固定了，而且传递函数还依赖闭包机制
type GetterFunc func(key string) ([]byte, error)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

type Group struct {
	name      string
	getter    Getter
	mainCache cache
	hotCache  cache
	peers     PeerPicker
	//singleflight并发请求控制
	loader *singleflight.Group
	//KeyStats映射
	keys map[string]*KeyStats
}

//封装一个原子类
type AtomicInt int64

//原子自增
func (i *AtomicInt) Add(n int64) {
	atomic.AddInt64((*int64)(i), n)
}

func (i *AtomicInt) Get() int64 {
	return atomic.LoadInt64((*int64)(i))
}

//Key的统计信息
type KeyStats struct {
	firstGetTime time.Time
	remoteCnt    AtomicInt //利用atomic包封装的原子类
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
		mainCache: cache{cacheBytes: cacheByte * 7 / 8},
		hotCache:  cache{cacheBytes: cacheByte / 8},
		loader:    &singleflight.Group{},
		keys:      map[string]*KeyStats{},
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
		log.Printf("[GaCache (mainCache)] hit")
		return v, nil
	}
	//add: hotCache
	if v, ok := g.hotCache.get(key); ok {
		log.Printf("[GaCache (hotCache)] hit")
		return v, nil
	}
	//当前节点没有数据,去其他地方加载
	return g.load(key)
}

func (g *Group) load(key string) (value ByteView, err error) {
	//放大缓存击穿效果
	//time.Sleep(100 * time.Millisecond)
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
	//远程获取cnt++
	if stat, ok := g.keys[key]; ok {
		stat.remoteCnt.Add(1)
		//计算QPS
		interval := float64(time.Now().Unix()-stat.firstGetTime.Unix()) / 60
		qps := stat.remoteCnt.Get() / int64(math.Max(1, math.Round(interval)))
		if qps >= maxMinuteRemoteQPS {
			//存入hotCache
			g.populateCache(key, ByteView{b: res.Value}, &g.hotCache)
			//删除映射关系,节省内存
			mu.Lock()
			delete(g.keys, key)
			mu.Unlock()
		}
	} else {
		//第一次获取
		g.keys[key] = &KeyStats{
			firstGetTime: time.Now(),
			remoteCnt:    1,
		}
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
	g.populateCache(key, value, &g.mainCache)
	return value, nil
}

//将从数据源获取的数据加入cache
//update: hotCache
func (g *Group) populateCache(key string, value ByteView, c *cache) {
	c.put(key, value)
}

func (g *Group) RegisterPeers(peers PeerPicker) {
	if g.peers != nil {
		panic("RegisterPeers called more than once ! ! !")
	}
	g.peers = peers
}
