package lru

import (
	"container/list"
)

type Cache struct {
	maxBytes  int64                         //最大可用内存,0代表无限制
	nbytes    int64                         //已使用内存
	ll        *list.List                    //双向链表
	cache     map[string]*list.Element      //key和list节点映射
	OnEvicted func(key string, value Value) //回调函数
}

//list里面存的kv结果
type entry struct {
	key   string
	value Value
}

//Value接口
type Value interface {
	Len() int //返回值占用内存大小
}

//New 最大可用内存0代表无限 key失效回调
func New(maxBytes int64, onEvicted func(key string, value Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

func (c *Cache) Get(key string) (value Value, ok bool) {
	if element, ok := c.cache[key]; ok {
		c.ll.MoveToFront(element)    //移动到队头,(go的源码看起来真舒服)
		kv := element.Value.(*entry) //强转成entry
		return kv.value, true
	}
	return nil, false
}

//从尾巴弹出节点
func (c *Cache) RemoveOldest() {
	tail := c.ll.Back()
	if tail != nil {
		c.ll.Remove(tail)
		kv := tail.Value.(*entry)
		delete(c.cache, kv.key)
		//key是string val是Value接口实现类
		c.nbytes -= int64(len(kv.key)) + int64(kv.value.Len())
		if c.OnEvicted != nil {
			//逐出key的回调函数
			c.OnEvicted(kv.key, kv.value)
		}
	}
}

//新增or修改
func (c *Cache) Put(key string, value Value) {
	if ele, ok := c.cache[key]; ok { //修改
		c.ll.MoveToFront(ele)
		kv := ele.Value.(*entry)
		c.nbytes += int64(value.Len()) - int64(kv.value.Len())
		kv.value = value
	} else {
		//新增的放到头部
		ele := c.ll.PushFront(&entry{key, value})
		c.cache[key] = ele
		c.nbytes += int64(len(key)) + int64(value.Len())
	}
	//内存不足
	for c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest() //按照LRU移除键
	}
}

func (c *Cache) Len() int {
	return c.ll.Len()
}
