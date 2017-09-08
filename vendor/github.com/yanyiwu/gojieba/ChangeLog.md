# ChangeLog

## v1.0.0

Notice:

+ remove NewExtractor, and use its api in Jieba.
+ upgrade cppjieba to v5.0.0

## v0.17.1

+ fix [issue 8](https://github.com/yanyiwu/gojieba/issues/8)

## v0.17.0

+ add ExtractWithWeight api and fix [issue 6](https://github.com/yanyiwu/gojieba/issues/6)

## v0.16.1

+ fix [issue 5](https://github.com/yanyiwu/gojieba/issues/5)

## v0.16.0

+ upgrade cppjieba -> v4.8.1 and support (word,freq,tag) format of user dict
+ add AddWord api

## v0.15.0

+ upgrade cppjieba -> v4.8.0, and make CutForSearch api behaves the same as jieba by Python
+ remove SetCutForSearchThreshold api

## v0.14.0

+ upgrade cppjieba -> v4.7.0
+ add new api: Tokenize for location information of words
+ add new api: SetCutForSearchThreshold
+ use Tokenize instead of Cut and SetCutForSearchThreshold(3) in gojieba/bleve

## v0.13.0

+ NewJieba(...string) support variable arguments
+ NewExtractor(...string) support variable arguments
+ removed example/ and write Example in `*_test.go`
+ add some kind of Benchmark, Testing
+ fix bug in extractor.go: calling C.free manully to free momery allocated by C.CString

## v0.12.0

+ supported [bleve] custom analyzer by using gojieba.

## v0.11.1

+ Fix bug: calling C.free manully to free momery allocated by C.CString.

## v0.11.0

+ Expose new api: Tag

## v0.10.3

Upgrade to fix potential trouble:

+ limonp -> v0.6.0
+ cppjieba -> v4.5.3

## v0.10.2

1. Fix error in `go vet` 
2. Upgrade limonp to v0.5.4 and cppjieba to v4.5.0 to support more unicode character

## v0.10.1

Upgrade:

+ cppjieba -> v4.4.1 to fix bug, see details in [CppJieba ChangeLog v4.4.1](https://github.com/yanyiwu/cppjieba/blob/master/ChangeLog.md#v441)

## v0.10.0

1. 源码布局变动，增加 deps/ 管理外部依赖代码。
2. 增加 Extractor 关键词抽取功能。
3. Upgrade [limonp] to version v0.5.1
4. Upgrade [cppjieba] to version v4.3.1
5. 分词接口变动 New -> NewJieba
6. 增加关键词抽取类 NewExtractor

## v0.9.3

1. 修复多余日志输出的问题。

## v0.9.2

1. 升级 [cppjieba] to v4.2.1  

## v0.9.1

1. 升级 [cppjieba] to v4.1.2  
2. 增加英文介绍 `README_EN.md`

## v0.9.0

1. 完成基本分词功能：【全模式，精确模式，搜索引擎模式】

[cppjieba]:https://github.com/yanyiwu/cppjieba
[limonp]:https://github.com/yanyiwu/limonp
[bleve]:https://github.com/blevesearch/bleve
