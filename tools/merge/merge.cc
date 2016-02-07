/*
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

#include "rocksdb/db.h"
#include "rocksdb/options.h"

#include <experimental/filesystem>

using namespace rocksdb;
namespace fs = std::experimental::filesystem;

int main(int argc, char* argv[]) {
  if(argc != 3) {
    std::cerr << "Wrong number of arguments\nusage : ./<executable>\
 <folder_having_rocksDB_directories_to_be_merged> <destination_folder>\n";
    exit(0);
  }
  
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

    rocksdb::Iterator* it = cur_db->NewIterator(rocksdb::ReadOptions());
    for (it->SeekToFirst(); it->Valid(); it->Next()) {
      Slice key_s = it->key();
      Slice val_s = it->value();    
      std::string val_t;
      Status s = db->Get(ReadOptions(), key_s, &val_t);
      if(s.ok()) { 
        assert(val_t == val_s.ToString() && "Same key has different value");
      } else {
        s = db->Put(WriteOptions(), key_s, val_s);
        assert(s.ok());
      }
    }
    assert(it->status().ok()); // Check for any errors found during the scan
    delete it;
    delete cur_db;
  }

  delete db;  
  return 0;
}
