[TOC]

## 整体流程

![mark](http://static.imlgw.top/blog/20200528/5UiOdagYdU2H.png?imageslim)

## LRU队列

因为缓存的数据都是在内存中的，内存资源是有限的，所以我们需要选择一种合适的策略，在内存快要满的时候剔除部分数据，这里选择了较为平衡且实现简单的方案LRU（最近最少使用）可以参考我之前的一篇博文 [LRU队列的实现（Java）](http://imlgw.top/2019/11/16/lrucache/) 这里为了方便，以及避免重复造轮子，直接使用`container` 包中的`List`双向链表和`map`来实现，具体代码见 [gacache/lru/lru.go](https://github.com/imlgw/gacache/blob/master/gacache/lru/lru.go)

## 并发控制

golang中的map并不是并发安全的容器，并发的访问可能会出现错误，所以我们需要加锁来控制并发的读写

```go
type cache struct {
    mu         sync.Mutex //互斥锁
    lru        *lru.Cache
    cacheBytes int64
}

func (c *cache) put(key string, value ByteView) {
    c.mu.Lock()
    defer c.mu.Unlock()
    if c.lru == nil { //尚未初始化,lazyinit
        c.lru = lru.New(c.cacheBytes, nil)
    }
    c.lru.Put(key, value)
}

func (c *cache) get(key string) (value ByteView, ok bool) {
    c.mu.Lock()
    defer c.mu.Unlock()
    if c.lru == nil {
        return
    }
    if v, ok := c.lru.Get(key); ok {
        return v.(ByteView), ok
    }
    return
}
```

## 一致性Hash

对于一个**分布式缓存**来讲，客户端在向某一个节点请求数据的时候，该节点应该如何去获取数据呢？是自己拿，还是去其他节点拿？如果这里不做处理，让当前节点自己去数据源取数据，那么最终可能每个节点都会缓存一份数据源的数据，这样不仅效率低下（需要和DB交互），而且浪费了很多空间，严格来说这就称不上是**分布式缓存**了，只能称之为**缓存集群**

所以我们需要一个方案能够将`key`和节点对应起来，有一种比较简单的方案就是将`key`哈希后对节点数量取余，这样每次请求都会打到同一个节点，但是这样的方案扩展性和容错性比较差，如果节点的数量发生变化可能会导致大量缓存的迁移，一瞬间大量缓存都失效了，这就可能导致缓存雪崩，所以这里我采用**一致性Hash算法**

### 实现

- 构造一个 `0 ~ 2^32-1` 大小的环
- 服务节点经过 hash 之后将定位到环中
- 将`key`哈希之后也定位到这个环中
- 顺时针找到离`hash(key)`最近的一个节点，也就是最终选择的缓存节点
- 考虑到服务节点的个数以及 hash 算法的问题导致环中的数据分布不均匀时引入了虚拟节点

**一致性Hash的Map**

```go
type Map struct {
    hash     Hash           //hash函数
    replicas int            //虚拟节点倍数
    keys     []int          //节点的地址,完整的协议/ip/port [eg. http://localhost:8001]
    hashMap  map[int]string //虚拟节点和真实节点映射关系
}
```

**添加机器/节点的方法**

```go
//添加机器/节点
func (m *Map) Add(keys ...string) {
    //hashMap := make(map[string][]int, len(keys))
    for _, key := range keys {
        //每台机器copy指定倍数的虚拟节点
        for i := 0; i < m.replicas; i++ {
            //计算虚拟节点的 hash值
            hash := int(m.hash([]byte(strconv.Itoa(i) + key)))
            //添加到环上
            m.keys = append(m.keys, hash)
            //hashMap[key] = append(hashMap[key], hash)
            //记录映射关系
            m.hashMap[hash] = key
        }
    }
    //fmt.Println(hashMap)
    //环上hash值进行排序
    sort.Ints(m.keys)
}
```

**Get获取节点**

因为整个环是有序的，所以可以直接通过二分去找第一个大于等于`hash(key)`的节点

```go
func (m *Map) Get(key string) string {
    if len(m.keys) == 0 {
        return ""
    }
    hash := int(m.hash([]byte(key)))
    //二分找第一个大于等于hash的节点idx
    idx := sort.Search(len(m.keys), func(i int) bool {
        return m.keys[i] >= hash
    })
    return m.hashMap[m.keys[idx%len(m.keys)]]
}
```

## 分布式节点通信

集群之间的通信通过Http协议，同时采用[Protobuf](https://github.com/protocolbuffers/protobuf)序列化数据提高传输效率

### Client端

通过节点地址和`groupName`以及`key`构成的地址请求数据，通过`protobuf`解码数据

```go
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
```

### Server端

实现`http.Handler`接口，对缓存中其他节点暴露地址，提供服务

```go
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
```

## 缓存击穿

一个存在的`key`突然失效，在失效的同时有大量的请求来请求这个`key`，这个时候大量请求就会直接打到DB，导致DB压力变大，甚至宕机

### 复现

这里简单的演示缓存击穿的效果，我们用下面的脚本启动4个`cache server` 并且在端口号为`8004`的`server`上启动一个前端服务，用于和客户端交互

```bash
start go run .  -port=8001
start go run .  -port=8002
start go run .  -port=8003
start go run .  -port=8004 -api=1
```

(windows平台的bat，其他平台可以用这个 [test.sh](https://github.com/imlgw/gacache/blob/master/test.sh))

尝试了用window的批处理写并发访问的脚本，但是效果不是很好，所以直接用`go`写了个小脚本测试并发请求

```go
var wg sync.WaitGroup

func main() {
    for i := 0; i < 5; i++ {
        wg.Add(1)
        go curl()
    }
    wg.Wait()
    fmt.Println("Done!")
}

func curl() {
    res, _ := http.Get("http://localhost:9999/api?key=resolmi")
    bytes, _ := ioutil.ReadAll(res.Body)
    fmt.Println(string(bytes))
    wg.Done()
}
```

开启5个`goroutine`，并发的去请求`resolmi`，此时`cache`肯定是没有这个key的，所以需要到DB中去取

然后就可以看到如下的情况：

![mark](http://static.imlgw.top/blog/20200529/pU2omvLdDqyQ.png?imageslim)

前台`Server`收到了Get "resolmi"请求，通过一致性Hash选择了远程节点`port:8003`

![mark](http://static.imlgw.top/blog/20200529/uovtpKufQME4.png?imageslim)

可以看到，5次请求全部打到了`SlowDB`中！！这就说明发生了缓存穿透！请求穿过了缓存层，打到DB

> 这里如果效果不明显可以尝试在`load`函数执行前加上一个`time.Sleep`，这样并发缓存击穿效果会更明显

### 解决方案

其实常见的方案就两种：

**1. 缓存永不过期**

缓存值不设置`ttl`，而是在`value`中添加一个逻辑的过期时间，这样请求就不会直接穿透到DB，同时我们可以通过当前时间判断该key是否过期，如果过期了就启动一个异步线程去更新缓存，这种方式用户延迟是最低的，但是可能会造成缓存的不一致

**2. 互斥锁**

在第一个请求获取数据的时候设置一个`mutex`互斥锁，让其他请求阻塞，当前第一个请求请求到数据返回之后释放`mutex`锁，其他请求停止阻塞，然后直接从缓存中获取数据

因为我们的项目本身是不支持`ttl`和删除操作的，所以第一种方案不太适合，所以采用第二种互斥锁的方案，实现了一个`singleflight`结构来处理缓存击穿

**封装请求call**

```go
//封装每个请求/调用
type call struct {
    wg  sync.WaitGroup
    val interface{} //请求的值
    err error       //err
}

//singleflight核心结构
type Group struct {
    mu sync.Mutex
    m  map[string]*call //key与call的映射
}
```

**并发控制核心代码**

```go
//并发请求控制
func (g *Group) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
    g.mu.Lock()
    if g.m == nil {
        g.m = make(map[string]*call)
    }
    if c, ok := g.m[key]; ok {
        g.mu.Unlock() //释放锁,按顺序进来
        c.wg.Wait()   //等着,等第一个请求完成
        return c.val, c.err
    }
    c := new(call)
    c.wg.Add(1)
    g.m[key] = c
    g.mu.Unlock()       //这里释放锁，让其他请求进入上面的分支中wait(其实只有并发量大的时候才会进入上面的分支)
    c.val, c.err = fn() //请求数据
    c.wg.Done()         //获取到值,第一个请求结束,其他请求可以获取到值了
    //删除m中的key,避免key发生变化,而取到的还是旧值
    g.mu.Lock()
    delete(g.m, key)
    g.mu.Unlock()
    return c.val, c.err
}
```

当多个并发的请求来请求同一个`key`的时候，只有**第一个请求**能拿到锁去DB中取数据，其他的请求就只能在方法外阻塞，当第一个请求构造好`call`就释放锁，这个时候其他并发的请求获取锁，进入阻塞的分支释放锁，然后再次阻塞，等待**第一个请求**获取到数据并封装进与`key`对应的`call`中，然后直接返回，不再向数据库请求，从而避免了缓存击穿

### 测试
用`singleFlight`包装一下我们的`load`方法

```go
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
```

再次使用上面同样的脚本进行测试

![mark](http://static.imlgw.top/blog/20200530/OThgxtFJrFIG.png?imageslim)

可以看到5个请求都成功了，我们再看看`log`

![mark](http://static.imlgw.top/blog/20200530/fsgxPPXNeVoo.png?imageslim)

可以看到，我请求了5次，但是实际上`getFromPeer`从远程节点取数据只执行了一次

![mark](http://static.imlgw.top/blog/20200530/ChE0FxjwlYbl.png?imageslim)

从`SlowDB`的查询也只执行了一次，说明我们的`singleFlight`并发控制生效了！

## 缓存穿透

查询一个不存在的数据，因为不存在则不会写到缓存中，所以每次都会去请求 DB，如果瞬间流量过大，穿透到 DB就会导致宕机

### 复现

使用上面的脚本再次启动`Cache Server`，然后用下面的go代码测试

```go
func main() {
    for i := 0; i < 5; i++ {
        wg.Add(1)
        //go curl()
        //i not exist
        go curlNotExist(strconv.Itoa(i)) //构造不用的key
    }
    wg.Wait()
    fmt.Println("Done!")
}

func curlNotExist(key string) {
    res, _ := http.Get("http://localhost:9999/api?key=" + key)
    bytes, _ := ioutil.ReadAll(res.Body)
    fmt.Println(string(bytes))
    wg.Done()
}
```

> 注意这里需要构造不同的key，相同的key会被前面的`singleFlight`组件拦截

结果如下：

```go
[ `go run .` | done: 1.8734423s ]
  1 not exist
  2 not exist
  0 not exist
  4 not exist
  3 not exist
  Done!
```

然后我们看看`Server`的情况

![mark](http://static.imlgw.top/blog/20200531/kV5ojfIeddm4.png?imageslim)

可以看到，5个`key`通过一致性Hash分散到不同的节点，但是由于`DB`中根本就没有这些数据，所以这些数据并不会缓存，每次查询都会到`DB`中去重新查询，如果短时间内有大量请求查询这些根本不存在的数据，那么这些请求都会直接打到DB层，将DB层压垮！

### 解决方案

**1.缓存空对象**

之所以发生缓存穿透，是因为缓存中没有存储这些空数据的key，导致这些请求全都打到数据库上。那么，我们可以稍微修改一下业务层的代码，将数据库查询结果为空的key也存储在缓存中。当后续又出现该key的查询请求时，缓存直接返回null，而无需查询数据库。

**2.布隆过滤器**

当业务层有查询请求的时候，首先去`BloomFilter`中查询该key是否存在。若不存在，则说明数据库中也不存在该数据，因此缓存都不要查了，直接返回null。若存在，则继续执行后续的流程，先前往缓存中查询，缓存中没有的话再前往数据库中的查询。

这里第一个种方案可以直接排除，一方面因为我们这个cache是不能删除数据的，只能被动的淘汰数据，缓存大量的空对象且得不到及时的删除会浪费大量内存，另一方面，缓存空对象的做法如果每次查询的不存在的key的不一样，那么这种方案也就起不到作用了

至于第二种方案，可行，但是不应该在缓存层来做，应该在业务层处理，也就是在上层处理，因为这是一个分布式的缓存组件，每个节点的数据都是不一样的，用布隆过滤器你只能判断在**当前节点**有没有，无法判断**远程节点**有没有，所以一开始就要将所有数据预热到布隆过滤器中，但是这样每一个节点都会需要一个布隆过滤器，这样做没有任何意义，所以缓存穿透的问题应该放到应用层去处理

## 部署方案



## TODO

- [x] 分布式节点通信
- [x] 一致性Hash
- [x] 缓存击穿
- [x] 布隆过滤器
- [ ] 热点互备