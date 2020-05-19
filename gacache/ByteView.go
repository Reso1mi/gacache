package gacache

//抽象一个只读的数据结构
//[]byte是切片，传递都是直接传递的指针，需要避免被修改，所以需要拷贝i一份
type ByteView struct {
	b []byte //包私有
}

//实现Value接口
func (v ByteView) Len() int {
	return len(v.b)
}

//返回一个数据切片
func (v ByteView) ByteSlice() []byte {
	return cloneBytes(v.b)
}

//以String的形式返回
func (v ByteView) String() string {
	return string(v.b)
}

func cloneBytes(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
