package models

type StatKV struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

func SumStatKV(kvs []StatKV) int64 {
	var t int64
	for _, k := range kvs {
		t += k.Count
	}
	return t
}
