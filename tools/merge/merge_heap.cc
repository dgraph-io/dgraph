/*
 * Copyright 2015 Manish R Jain <manishrjain@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 *  compile : g++ rocks_merge.cc <path_to_rocksDB_installation>/librocksdb.so.4.1 --std=c++11 -lstdc++fs
 *  usage : ./<executable> <folder_having_rocksDB_directories_to_be_merged> <destination_folder>
 *
 */

#include <fstream>
#include <cstdio>
#include <iostream>
#include <string>
#include <queue>
#include <vector>

#include "rocksdb/db.h"
#include "rocksdb/options.h"

#include <experimental/filesystem>

using namespace rocksdb;
namespace fs = std::experimental::filesystem;

class node {
public:
  Slice key;
  Slice value;
  int idx;
  node(Slice k, Slice v, int id) {
    key = k;
    value = v;
    idx = id;
  }
};

class compare {
  public:
    bool operator()(node &a, node &b) {
      return a.key.compare(b.key) <= 0;
    }
};

int main(int argc, char* argv[]) {
  if(argc != 3) {
    std::cerr << "Wrong number of arguments\nusage : ./<executable>\
        <folder_having_rocksDB_directories_to_be_merged> <destination_folder>\n";
    exit(0);
  }

  int counter = 0;
  std::priority_queue<struct node, std::vector<node>, compare> pq;
  std::vector<rocksdb::Iterator*> itVec;
  std::string destinationDB = argv[2], mergeDir = argv[1];
  DB* db;
  Options options;
  // Optimize RocksDB. This is the easiest way to get RocksDB to perform well
  options.IncreaseParallelism();
  options.OptimizeLevelStyleCompaction();
  // create the DB if it's not already present
  options.create_if_missing = true;

  // open DB
  Status s = DB::Open(options, destinationDB, &db);
  assert(s.ok());

  for (auto& dirEntry : fs::directory_iterator(mergeDir)) {
    std::cout << dirEntry << std::endl;
    DB* cur_db;
    Options options;
    options.IncreaseParallelism();
    options.OptimizeLevelStyleCompaction();
    // Don't create the DB if it's not already present
    options.create_if_missing = false;

    // open DB
    Status s1 = DB::Open(options, dirEntry.path().c_str(), &cur_db);
    assert(s1.ok());

    rocksdb::Iterator *it = cur_db->NewIterator(rocksdb::ReadOptions());
    it->SeekToFirst();
    if(!it->Valid()) {
      continue;
    } 
    struct node tnode(it->key(), it->value(), counter++);
    itVec.push_back(it);
    pq.push(tnode);
  }

  Slice lastKey, lastValue;

  while(!pq.empty()) {
    const struct node &top = pq.top();
    pq.pop();

    if(top.key == lastKey) {
      assert(top.value == lastValue);
    } else {
      s = db->Put(WriteOptions(), top.key, top.value);
      assert(s.ok());
      lastKey = top.key;
      lastValue = top.value;
    }

    itVec[top.idx]->Next();
    if(!itVec[top.idx]->Valid()) {
      continue;
    }
    struct node tnode(itVec[top.idx]->key(), itVec[top.idx]->value(), top.idx);
    pq.push(tnode);
  }

  delete db;
  return 0;
}
