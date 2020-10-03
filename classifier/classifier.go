// Package classifier provider a bayesian classifier.
package classifier

import (
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/xmdhs/bayesian-classifier/storage"
	"github.com/xmdhs/bayesian-classifier/util"
)

// Classifier is a bayesian classifier, provide training score categorize methods and http api.
type Classifier struct {
	segmenter     *util.Segmenter      // 分词器
	defaultProb   float64              // 单词在某一分类中出现的默认概率（不存在时）
	defaultWeight float64              // 默认概率的权重
	debug         bool                 // 是否开启调试
	storage       *storage.FileStorage // 存储引擎
	data          *data                // 存储数据
}

type data struct {
	Categorys map[string]float64            `json:"category"` // 分类数据
	Words     map[string]map[string]float64 `json:"words"`    // 单词数据
}

// NewClassifier 实例化一个分类器
// 要求以一个字典的格式传入配置信息和分词器
func NewClassifier(config map[string]interface{}) *Classifier {
	t := new(Classifier)
	// 配置信息
	t.defaultProb = config["defaultProb"].(float64)
	t.defaultWeight = config["defaultWeight"].(float64)
	t.debug = config["debug"].(bool)

	// 初始化数据结构
	t.data = new(data)
	t.data.Categorys = make(map[string]float64)
	t.data.Words = make(map[string]map[string]float64)

	// 初始化存储器
	var err error
	storageConfig := config["storage"].(map[string]string)
	t.storage, err = storage.NewFileStorage(storageConfig["path"])
	if err != nil {
		log.Fatalln("存储器初始化失败：", err)
	}

	// 初始化分词器
	t.segmenter = util.NewSegmenter()

	// 加载数据
	log.Println("加载数据", storageConfig["path"])
	t.Import()

	// 自动保存数据
	frequency, _ := strconv.Atoi(storageConfig["frequency"])
	if frequency > 0 {
		log.Println("开启自动数据自动保存")
		go func() {
			var err error
			for {
				time.Sleep(time.Second * time.Duration(frequency))
				err = t.Export()
				if err != nil {
					log.Println("自动保存数据失败：", err)
					runtime.Goexit()
				}
				log.Println("自动保存数据成功")
			}
		}()
	}

	// 初始化HTTP服务
	enableHTTP := config["http"].(bool)
	if enableHTTP {
		port := config["httpPort"].(string)
		log.Println("Http服务器启动", port)
		httpServ := NewHTTP(port, t)
		go httpServ.Start()
	} else {
		log.Println("Http服务未启动")
	}

	log.Println("初始化完成.\n")
	return t
}

// Training 训练分类器
func (t *Classifier) Training(doc, category string) {
	doc = strings.TrimSpace(doc)
	category = strings.TrimSpace(category)
	if doc == "" || category == "" {
		log.Println("提供了空文档")
		return
	}

	// 更新单词数据
	// 同一个文档中单词出现多次，仅记录一次
	fwords := make(map[string]bool)
	words := t.segmenter.Segment(doc)
	for _, word := range words {
		if _, ok := fwords[word]; ok {
			continue
		}
		fwords[word] = true
		if _, ok := t.data.Words[word]; !ok {
			t.data.Words[word] = make(map[string]float64)
		}
		t.data.Words[word][category]++
		log.Println("单词训练：", word)
	}
	// 更新分类统计
	t.data.Categorys[category]++

	return
}

func (t *Classifier) TrainingwithSlash(doc, category string) {
	doc = strings.TrimSpace(doc)
	category = strings.TrimSpace(category)
	if doc == "" || category == "" {
		log.Println("提供了空文档")
		return
	}

	// 更新单词数据
	// 同一个文档中单词出现多次，仅记录一次
	fwords := make(map[string]bool)
	words := strings.Split(doc, "/")
	for _, word := range words {
		if _, ok := fwords[word]; ok {
			continue
		}
		fwords[word] = true
		if _, ok := t.data.Words[word]; !ok {
			t.data.Words[word] = make(map[string]float64)
		}
		t.data.Words[word][category]++
		log.Println("单词训练：", word)
	}
	// 更新分类统计
	t.data.Categorys[category]++

	return
}

// Score 查看一个单词的概率分布
func (t *Classifier) Score(word, category string) []*ScoreItem {
	scores := NewScores()
	if _, ok := t.data.Words[word]; !ok {
		return scores.GetSlice()
	}

	// 指定分类
	if category != "" {
		scores.Append(category, t.wordWeightProb(word, category, t.defaultWeight, t.defaultProb))
	} else {
		// 计算所有分类
		for category = range t.data.Words[word] {
			scores.Append(category, t.wordWeightProb(word, category, t.defaultWeight, t.defaultProb))
		}
	}
	return scores.Top(10)
}

// 单词在指定分类所有文档中出现的概率为
func (t *Classifier) wordProb(word, category string) float64 {
	if _, ok := t.data.Words[word]; !ok {
		return 0.0
	}
	if num, ok := t.data.Words[word][category]; ok {
		return num / t.data.Categorys[category]
	}
	return 0.0
}

// 单词在指定分类所有文档中出现的概率，加权权重
// 当不存在该分类的概率时，推荐假定为：0.5，即：assumedprob = 0.5
// 假定概率赋予的权重推荐为1，代表假定概率的权重与一个单词的权重相当
func (t *Classifier) wordWeightProb(word, category string, weight, assumedprob float64) float64 {
	// 计算当前的概率
	basicProb := t.wordProb(word, category)
	// 统计单词在所有分类中出现的次数
	var total float64
	for _, num := range t.data.Words[word] {
		total += num
	}
	// 计算加权平均概率
	return ((weight * assumedprob) + (total * basicProb)) / (weight + total)
}

// Categorize 对文档分类
// P(category|document) = P(document|category) * P(category) / P(document)
func (t *Classifier) Categorize(doc string) []*ScoreItem {
	scores := NewScores()
	total := t.categoryNumTotal()
	for cate := range t.data.Categorys {
		scores.Append(cate, t.docProb(doc, cate)*t.data.Categorys[cate]/total)
	}
	return scores.Top(10)
}

// docProb 整篇文档的概率计算
// P(document|category) = P(word1|category) * P(word2|category) ...
func (t *Classifier) docProb(doc, category string) float64 {
	prob := 1.0
	// 分词，获取逐个单词指定分类的概率
	words := t.segmenter.Segment(doc)
	for _, word := range words {
		wp := t.wordWeightProb(word, category, t.defaultWeight, t.defaultProb)
		prob *= wp
	}
	return prob
}

// categoryNumTotal 获取所有单词训练的数量
func (t *Classifier) categoryNumTotal() float64 {
	total := 0.0
	for _, n := range t.data.Categorys {
		total += n
	}
	return total
}

// Categorys 获取所有的分类数据
func (t *Classifier) Categorys() map[string]float64 {
	return t.data.Categorys
}

// Export 导出训练数据
func (t *Classifier) Export() error {
	return t.storage.Save(t.data)
}

// Import 导入训练数据
func (t *Classifier) Import() error {
	return t.storage.Load(t.data)
}
