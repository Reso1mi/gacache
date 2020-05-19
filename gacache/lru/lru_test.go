package lru

import (
	"fmt"
	"reflect"
	"testing"
)

type String string

func (str String) Len() int {
	return len(str)
}

func TestCache_Get(t *testing.T) {
	lru := New(int64(0), nil)
	lru.Put("key1", String("value1"))
	fmt.Println(lru.Get("key1"))
	if val, ok := lru.Get("key1"); !ok || string(val.(String)) != "value1" {
		t.Fatalf("cache key1=value fail!!!")
	}
}

func TestCache_Put(t *testing.T) {
	k1, k2, k3 := "key1", "key2", "key3"
	v1, v2, v3 := "value1", "value2", "value3"
	cap := len(k1 + k2 + v1 + v2)
	lru := New(int64(cap), nil)
	lru.Put(k1, String(v1))
	lru.Put(k2, String(v2))
	lru.Put(k3, String(v3)) //插入k3的时候k1会被移除
	if _, ok := lru.Get("key1"); ok || lru.Len() != 2 {
		t.Fatalf("remove key1 fail")
	}
}

func TestOnEvicted(t *testing.T) {
	keys := make([]string, 0)
	callback := func(key string, value Value) {
		keys = append(keys, key)
	}
	k1, k2, k3 := "key1", "key2", "key3"
	v1, v2, v3 := "汉字", "value2", "value3"
	lru := New(int64(12), callback)
	lru.Put(k1, String(v1)) //插入k1后占用10byte
	if lru.nbytes != 10 {
		t.Fatalf("error")
	}
	lru.Put(k2, String(v2)) //插入k2后k1会被移除
	lru.Put(k3, String(v3)) //插入k3的时候k2会被移除
	fmt.Println(lru.maxBytes, lru.nbytes)
	expect := []string{"key1", "key2"}
	if !reflect.DeepEqual(expect, keys) {
		t.Fatalf("Call onevicted failed expect:%s , keys: %s", expect, keys)
	}
}
