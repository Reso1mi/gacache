package main

import (
	"fmt"
	"log"
	"reflect"
	"testing"
)

var db = map[string]string{
	"Tom":     "123",
	"Resolmi": "20",
	"Sam":     "Nice",
}

func TestGetter(t *testing.T) {
	var f Getter = GetterFunc(func(key string) ([]byte, error) {
		return []byte(key), nil
	})

	expect := []byte("key")
	if v, _ := f.Get("key"); !reflect.DeepEqual(v, expect) {
		t.Fatalf("callback fail!!")
	}
}

func TestGet(t *testing.T) {
	loadCounts := make(map[string]int, len(db))
	gac := NewGroup("scores", 2<<10, GetterFunc(func(key string) ([]byte, error) {
		log.Println("[DB] search key", key)
		if v, ok := db[key]; ok {
			if _, ok := loadCounts[key]; !ok {
				loadCounts[key] = 0
			}
			loadCounts[key]++ //统计加载次数
			return []byte(v), nil
		}
		return nil, fmt.Errorf("%s not exist", key)
	}))
	for k, v := range db {
		//这里取首先肯定是取不到，取不到就会调用上面的回调函数取db中取
		if view, err := gac.Get(k); err != nil || view.String() != v {
			t.Fatalf("failed to get value of tom")
		}
		//再取一次看看，如果loadCount大于1就说明重复加载了，缓存没生效
		if _, err := gac.Get(k); err != nil || loadCounts[k] > 1 {
			t.Fatalf("cache %s miss", k)
		}
	}
	//key不存在的情况
	if view, err := gac.Get("unknown"); err == nil {
		t.Fatalf("the value of unknow should be empty, but %s got", view)
	}
}
