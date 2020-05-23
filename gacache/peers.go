package gacache

//顾名思义，节点选择接口
type PeerPicker interface {
	PickPeer(key string) (peer PeerGetter, ok bool)
}

//节点获取数据的接口
type PeerGetter interface {
	Get(group string, key string) ([]byte, error)
}
