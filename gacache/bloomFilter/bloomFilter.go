package bloomFilter

import (
	"github.com/willf/bloom"
	"math"
)

//n:数据量 fpp:误判率
func New(n uint, fpp float64) *bloom.BloomFilter {
	if fpp < 0 {
		panic("invalid fpp!!!fpp should be greater than 0.0")
	}
	//根据数据量n误判率推算出位向量长度 m = (-n*lnp)/(ln2)^2
	m := uint((-float64(n) * math.Log(fpp)) / (math.Ln2 * math.Ln2))
	//根据数据量n和位向量长度m计算hash函数的个数 k = (m/n) * ln2
	k := uint(math.Max(1, math.Round(float64(m)/float64(n)*math.Ln2)))
	return bloom.New(m, k)
}
