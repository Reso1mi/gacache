package main

import (
	"fmt"
	"gacache"
	"log"
	"sort"
)

var db = map[string]string{
	"tom":     "110",
	"resolmi": "20",
	"jerry":   "119",
}

func main() {
	//Http测试
	gacache.NewGroup("scores", 2<<10, gacache.GetterFunc(func(key string) ([]byte, error) {
		log.Println("[DB] search key", key)
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("%s not exist", key)
	}))
	addr := "localhost:9999"
	//peers := gacache.NewHTTPPool(addr)
	log.Println("gacache is running at", addr)
	//log.Fatal(http.ListenAndServe(addr, peers))
	a := []int{0, -22, 41, 5, 5, 6}
	sort.Ints(a)
	fmt.Println(a)
}
