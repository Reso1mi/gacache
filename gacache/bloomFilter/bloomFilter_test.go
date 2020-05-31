package bloomFilter

import (
	"fmt"
	"strconv"
	"testing"
)

const size = 1000000

func TestBloom(t *testing.T) {
	filter := New(size, 0.01)
	for i := 0; i < size; i++ {
		filter.Add([]byte(strconv.Itoa(i)))
	}
	for i := 0; i < size; i++ {
		if !filter.Test([]byte(strconv.Itoa(i))) {
			t.Fatalf("judge error !  key:=%v is contain", i)
		}
	}
	count := 0
	for i := size; i < size+10000; i++ {
		if filter.Test([]byte(strconv.Itoa(i))) {
			count++
		}
	}
	fmt.Printf("误判数量：%d,误判概率：%f\n", count, float32(count)/10000)
}
