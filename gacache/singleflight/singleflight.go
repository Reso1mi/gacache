package singleflight

import "sync"

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
