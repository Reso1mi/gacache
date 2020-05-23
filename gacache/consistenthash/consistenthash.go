package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

type Hash func(data []byte) uint32

type Map struct {
	hash     Hash           //hash函数
	replicas int            //虚拟节点倍数
	keys     []int          //keys,完整的 协议/ip/port [eg. http://localhost:8004]
	hashMap  map[int]string //虚拟节点和真实节点映射
}

func New(replicas int, fn Hash) *Map {
	m := &Map{
		hash:     fn,
		replicas: replicas,
		hashMap:  make(map[int]string),
	}
	if m.hash == nil {
		//默认的hash方法crc32
		m.hash = crc32.ChecksumIEEE
	}
	return m
}

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
