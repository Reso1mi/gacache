package main

import "fmt"

type Value struct {
	key string
}

func main() {
	var a interface{} = &Value{
		key: "dsadsa",
	}
	value := a.(*Value)
	fmt.Println(value)

	fmt.Println(int64(0))
}
