# GoJieba [English](README_EN.md)

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

[GoJieba]是"结巴"中文分词的Golang语言版本。

## 简介

+ 支持多种分词方式，包括: 最大概率模式, HMM新词发现模式, 搜索引擎模式, 全模式
+ 核心算法底层由C++实现，性能高效。
+ 无缝集成到 [bleve] 到进行搜索引擎的中文分词功能。
+ 字典路径可配置，NewJieba(...string), NewExtractor(...string) 可变形参，当参数为空时使用默认词典(推荐方式)

## 用法

```
go get github.com/yanyiwu/gojieba
```

分词示例

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

```
我来到北京清华大学
全模式: 我/来到/北京/清华/清华大学/华大/大学
我来到北京清华大学
精确模式: 我/来到/北京/清华大学
比特币
精确模式: 比特/币
比特币
添加词典后,精确模式: 比特币
他来到了网易杭研大厦
新词识别: 他/来到/了/网易/杭研/大厦
小明硕士毕业于中国科学院计算所，后在日本京都大学深造
搜索引擎模式: 小明/硕士/毕业/于/中国/科学/学院/科学院/中国科学院/计算/计算所/，/后/在/日本/京都/大学/日本京都大学/深造
长春市长春药店
词性标注: 长春市/ns,长春/ns,药店/n
区块链
词性标注: 区块链/nz
长江大桥
搜索引擎模式: 长江/大桥/长江大桥
长江大桥
Tokenize: [{长江 0 6} {大桥 6 12} {长江大桥 0 12}]
```

See example in [jieba_test](jieba_test.go), [extractor_test](extractor_test.go)

## Bleve 中文分词插件用法

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

## 性能评测

[Jieba中文分词系列性能评测]

## 测试

Unittest

```
go test ./...
```

Benchmark

```
go test -bench "Jieba" -test.benchtime 10s
go test -bench "Extractor" -test.benchtime 10s
```

## 客服

+ Email: `i@yanyiwu.com`
+ QQ: 64162451
+ WeChat: 
+ ![image](http://7viirv.com1.z0.glb.clouddn.com/5a7d1b5c0d_yanyiwu_personal_qrcodes.jpg)

[CppJieba]:http://github.com/yanyiwu/cppjieba
[GoJieba]:http://github.com/yanyiwu/gojieba
[Jieba]:https://github.com/fxsjy/jieba
[Jieba中文分词系列性能评测]:http://yanyiwu.com/work/2015/06/14/jieba-series-performance-test.html
[bleve]:https://github.com/blevesearch/bleve

[![Bitdeli Badge](https://d2weczhvl823v0.cloudfront.net/yanyiwu/gojieba/trend.png)](https://bitdeli.com/free "Bitdeli Badge")
