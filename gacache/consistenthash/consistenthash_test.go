package consistenthash

import (
	"strconv"
	"testing"
)

func TestHashing(t *testing.T) {
	hash := New(3, func(key []byte) uint32 {
		//自定义hash直接返回原节点数字编号
		i, _ := strconv.Atoi(string(key))
		return uint32(i)
	})
	//2: 02 12 22
	//4: 04 14 24
	//6: 06 16 26
	//keys: 2 4 6 12 14 16 22 24 26
	hash.Add("2", "4", "6")
	testCase := map[string]string{
		"2":  "2",
		"13": "4",
		"15": "6",
		"27": "2",
	}

	for k, v := range testCase {
		if hash.Get(k) != v {
			t.Errorf("Ask %s,response %s, should be %s !!!", k, hash.Get(k), v)
		}
	}

	//08 18 28
	hash.Add("8")
	testCase["27"] = "8"

	for k, v := range testCase {
		if hash.Get(k) != v {
			t.Errorf("Ask %s,should be %s !!!", k, v)
		}
	}
}
