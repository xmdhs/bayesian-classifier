package util

import (
	"strings"

	"github.com/yanyiwu/gojieba"
)

type Segmenter struct {
	segmenter *gojieba.Jieba
}

func NewSegmenter() *Segmenter {
	return &Segmenter{segmenter: gojieba.NewJieba(`dict/jieba.dict.utf8`, `dict/hmm_model.utf8`, `dict/user.dict.utf8`, `dict/idf.utf8`, `dict/stop_words.utf8`)}
}

// 分词
func (t *Segmenter) Segment(text string) []string {
	output := t.segmenter.CutForSearch(text, true)
	return filterWord(output)
}

// 过滤干扰词 空格，单字词
func filterWord(ws []string) []string {
	result := make([]string, 0)
	for _, w := range ws {
		if strings.Count(strings.TrimSpace(w), "") <= 2 {
			continue
		}
		result = append(result, w)
	}
	return result
}
