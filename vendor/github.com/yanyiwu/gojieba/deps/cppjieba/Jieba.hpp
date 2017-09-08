#ifndef CPPJIEAB_JIEBA_H
#define CPPJIEAB_JIEBA_H

#include "QuerySegment.hpp"
#include "KeywordExtractor.hpp"

namespace cppjieba {

class Jieba {
 public:
  Jieba(const string& dict_path, 
        const string& model_path,
        const string& user_dict_path, 
        const string& idfPath, 
        const string& stopWordPath) 
    : dict_trie_(dict_path, user_dict_path),
      model_(model_path),
      mp_seg_(&dict_trie_),
      hmm_seg_(&model_),
      mix_seg_(&dict_trie_, &model_),
      full_seg_(&dict_trie_),
      query_seg_(&dict_trie_, &model_),
      extractor(&dict_trie_, &model_, idfPath, stopWordPath) {
  }
  ~Jieba() {
  }

  struct LocWord {
    string word;
    size_t begin;
    size_t end;
  }; // struct LocWord

  void Cut(const string& sentence, vector<string>& words, bool hmm = true) const {
    mix_seg_.Cut(sentence, words, hmm);
  }
  void Cut(const string& sentence, vector<Word>& words, bool hmm = true) const {
    mix_seg_.Cut(sentence, words, hmm);
  }
  void CutAll(const string& sentence, vector<string>& words) const {
    full_seg_.Cut(sentence, words);
  }
  void CutAll(const string& sentence, vector<Word>& words) const {
    full_seg_.Cut(sentence, words);
  }
  void CutForSearch(const string& sentence, vector<string>& words, bool hmm = true) const {
    query_seg_.Cut(sentence, words, hmm);
  }
  void CutForSearch(const string& sentence, vector<Word>& words, bool hmm = true) const {
    query_seg_.Cut(sentence, words, hmm);
  }
  void CutHMM(const string& sentence, vector<string>& words) const {
    hmm_seg_.Cut(sentence, words);
  }
  void CutHMM(const string& sentence, vector<Word>& words) const {
    hmm_seg_.Cut(sentence, words);
  }
  void CutSmall(const string& sentence, vector<string>& words, size_t max_word_len) const {
    mp_seg_.Cut(sentence, words, max_word_len);
  }
  void CutSmall(const string& sentence, vector<Word>& words, size_t max_word_len) const {
    mp_seg_.Cut(sentence, words, max_word_len);
  }
  
  void Tag(const string& sentence, vector<pair<string, string> >& words) const {
    mix_seg_.Tag(sentence, words);
  }
  string LookupTag(const string &str) const {
    return mix_seg_.LookupTag(str);
  }
  bool InsertUserWord(const string& word, const string& tag = UNKNOWN_TAG) {
    return dict_trie_.InsertUserWord(word, tag);
  }

  void ResetSeparators(const string& s) {
    //TODO
    mp_seg_.ResetSeparators(s);
    hmm_seg_.ResetSeparators(s);
    mix_seg_.ResetSeparators(s);
    full_seg_.ResetSeparators(s);
    query_seg_.ResetSeparators(s);
  }

  const DictTrie* GetDictTrie() const {
    return &dict_trie_;
  } 
  const HMMModel* GetHMMModel() const {
    return &model_;
  }

 private:
  DictTrie dict_trie_;
  HMMModel model_;
  
  // They share the same dict trie and model
  MPSegment mp_seg_;
  HMMSegment hmm_seg_;
  MixSegment mix_seg_;
  FullSegment full_seg_;
  QuerySegment query_seg_;

 public:
  KeywordExtractor extractor;
}; // class Jieba

} // namespace cppjieba

#endif // CPPJIEAB_JIEBA_H
