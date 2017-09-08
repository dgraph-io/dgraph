# GoJieba [简体中文](README.md)

[![Build Status](https://travis-ci.org/yanyiwu/gojieba.png?branch=master)](https://travis-ci.org/yanyiwu/gojieba) 
[![Author](https://img.shields.io/badge/author-@yanyiwu-blue.svg?style=flat)](http://yanyiwu.com/) 
[![Performance](https://img.shields.io/badge/performance-excellent-brightgreen.svg?style=flat)](http://yanyiwu.com/work/2015/06/14/jieba-series-performance-test.html) 
[![License](https://img.shields.io/badge/license-MIT-yellow.svg?style=flat)](http://yanyiwu.mit-license.org)
[![GoDoc](https://godoc.org/github.com/yanyiwu/gojieba?status.svg)](https://godoc.org/github.com/yanyiwu/gojieba)
[![Coverage Status](https://coveralls.io/repos/yanyiwu/gojieba/badge.svg?branch=master&service=github)](https://coveralls.io/github/yanyiwu/gojieba?branch=master)
[![codebeat badge](https://codebeat.co/badges/a336d042-3583-4212-8204-88da4407438e)](https://codebeat.co/projects/github-com-yanyiwu-gojieba)
[![Go Report Card](https://goreportcard.com/badge/yanyiwu/gojieba)](https://goreportcard.com/report/yanyiwu/gojieba)
[![Awesome](https://cdn.rawgit.com/sindresorhus/awesome/d7305f38d29fed78fa85652e3a63e154dd8e8829/media/badge.svg)](https://github.com/avelino/awesome-go) 

[![logo](http://7viirv.com1.z0.glb.clouddn.com/GoJieBaLogo-v2.png)](http://yanyiwu.com/work/2015/09/14/c-cpp-go-mix-programming.html)

[GoJieba] is a Jieba Chinese Word Segmentation lib written by Go。

## Usage

```
go get github.com/yanyiwu/gojieba
```

Chinese Word Segmentation Example:

```
package main

import (
	"fmt"
	"strings"

	"github.com/yanyiwu/gojieba"
)

func main() {
	var s string
	var words []string
	use_hmm := true
	x := gojieba.NewJieba()
	defer x.Free()

	s = "我来到北京清华大学"
	words = x.CutAll(s)
	fmt.Println(s)
	fmt.Println("全模式:", strings.Join(words, "/"))

	words = x.Cut(s, use_hmm)
	fmt.Println(s)
	fmt.Println("精确模式:", strings.Join(words, "/"))
	s = "比特币"
	words = x.Cut(s, use_hmm)
	fmt.Println(s)
	fmt.Println("精确模式:", strings.Join(words, "/"))

	x.AddWord("比特币")
	s = "比特币"
	words = x.Cut(s, use_hmm)
	fmt.Println(s)
	fmt.Println("添加词典后,精确模式:", strings.Join(words, "/"))

	s = "他来到了网易杭研大厦"
	words = x.Cut(s, use_hmm)
	fmt.Println(s)
	fmt.Println("新词识别:", strings.Join(words, "/"))

	s = "小明硕士毕业于中国科学院计算所，后在日本京都大学深造"
	words = x.CutForSearch(s, use_hmm)
	fmt.Println(s)
	fmt.Println("搜索引擎模式:", strings.Join(words, "/"))

	s = "长春市长春药店"
	words = x.Tag(s)
	fmt.Println(s)
	fmt.Println("词性标注:", strings.Join(words, ","))

	s = "区块链"
	words = x.Tag(s)
	fmt.Println(s)
	fmt.Println("词性标注:", strings.Join(words, ","))

	s = "长江大桥"
	words = x.CutForSearch(s, !use_hmm)
	fmt.Println(s)
	fmt.Println("搜索引擎模式:", strings.Join(words, "/"))

	wordinfos := x.Tokenize(s, gojieba.SearchMode, !use_hmm)
	fmt.Println(s)
	fmt.Println("Tokenize:(搜索引擎模式)", wordinfos)

	wordinfos = x.Tokenize(s, gojieba.DefaultMode, !use_hmm)
	fmt.Println(s)
	fmt.Println("Tokenize:(默认模式)", wordinfos)

	keywords := x.ExtractWithWeight(s, 5)
	fmt.Println("Extract:", keywords)
}
```


See example in [jieba_test](jieba_test.go), [extractor_test](extractor_test.go)

## Bleve Plugin Usage

```
package main

import (
	"fmt"
	"os"

	"github.com/blevesearch/bleve"
	"github.com/yanyiwu/gojieba"
	_ "github.com/yanyiwu/gojieba/bleve"
)

func Example() {
	INDEX_DIR := "gojieba.bleve"
	messages := []struct {
		Id   string
		Body string
	}{
		{
			Id:   "1",
			Body: "你好",
		},
		{
			Id:   "2",
			Body: "世界",
		},
		{
			Id:   "3",
			Body: "亲口",
		},
		{
			Id:   "4",
			Body: "交代",
		},
	}

	indexMapping := bleve.NewIndexMapping()
	os.RemoveAll(INDEX_DIR)
	// clean index when example finished
	defer os.RemoveAll(INDEX_DIR)

	err := indexMapping.AddCustomTokenizer("gojieba",
		map[string]interface{}{
			"dictpath":     gojieba.DICT_PATH,
			"hmmpath":      gojieba.HMM_PATH,
			"userdictpath": gojieba.USER_DICT_PATH,
			"idf":          gojieba.IDF_PATH,
			"stop_words":   gojieba.STOP_WORDS_PATH,
			"type":         "gojieba",
		},
	)
	if err != nil {
		panic(err)
	}
	err = indexMapping.AddCustomAnalyzer("gojieba",
		map[string]interface{}{
			"type":      "gojieba",
			"tokenizer": "gojieba",
		},
	)
	if err != nil {
		panic(err)
	}
	indexMapping.DefaultAnalyzer = "gojieba"

	index, err := bleve.New(INDEX_DIR, indexMapping)
	if err != nil {
		panic(err)
	}
	for _, msg := range messages {
		if err := index.Index(msg.Id, msg); err != nil {
			panic(err)
		}
	}

	querys := []string{
		"你好世界",
		"亲口交代",
	}

	for _, q := range querys {
		req := bleve.NewSearchRequest(bleve.NewQueryStringQuery(q))
		req.Highlight = bleve.NewHighlight()
		res, err := index.Search(req)
		if err != nil {
			panic(err)
		}
		fmt.Println(res)
	}
}

func main() {
	Example()
}
```

Output:

```
2 matches, showing 1 through 2, took 360.584µs
    1. 2 (0.423287)
    Body
        <mark>世界</mark>
    2. 1 (0.423287)
    Body
        <mark>你好</mark>

2 matches, showing 1 through 2, took 131.055µs
    1. 4 (0.423287)
    Body
        <mark>交代</mark>
    2. 3 (0.423287)
    Body
        <mark>亲口</mark>
```

See example in [bleve_test](bleve/bleve_test.go)

## Performance

[GoJieba] has a good enough performance,
it maybe is the best of all the Chinese Word Segmentation lib  from the angle of high performance.

Please see more details in [jieba-performance-comparison],
but the article is written by Chinese, Maybe someday it will be transferred to English.

## Contact

```
i@yanyiwu.com
```

[CppJieba]:http://github.com/yanyiwu/cppjieba
[GoJieba]:http://github.com/yanyiwu/gojieba
[jieba-performance-comparison]:http://yanyiwu.com/work/2015/06/14/jieba-series-performance-test.html
[Jieba]:https://github.com/fxsjy/jieba

