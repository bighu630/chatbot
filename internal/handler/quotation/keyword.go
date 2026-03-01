package quotation

import "sync"

type wordType string

const (
	insult       wordType = "mata"    // 骂人
	simp         wordType = "tiangou" // 舔狗
	anxiety      wordType = "psycho"  // 神经
	couple       wordType = "cp"      // couple
	kfc          wordType = "kfc"     // fkc
	neteaseCloud wordType = "wyy"     // 网易云
)

// 默认支持这些，但是
var defaultWordTypt = []wordType{insult, simp, anxiety, couple, kfc, neteaseCloud}

type QuotationManager struct {
	// 读写锁
	mu sync.RWMutex
	// keyword类型列表
	wordTypeList []wordType
	// 关键词转 类型
	keyToType map[string]wordType
}

// 返回所有类型
func (q *QuotationManager) GetAllType() []string {
	t := make([]string, len(q.wordTypeList))
	for i, k := range q.wordTypeList {
		t[i] = string(k)
	}
	return t
}

func (q *QuotationManager) GetRangeOneByType(t string) (string, error) {
	return "", nil
}
