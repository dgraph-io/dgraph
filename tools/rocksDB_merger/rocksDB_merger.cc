/*
 *	Author : Ashwin <ashwin2007ray@gmail.com>
 *
 *	compile : g++ rocks_merge.cc <path_to_rocksDB_installation>/librocksdb.so.4.1 --std=c++11 -lstdc++fs
 *	usage : ./<executable> <folder_having_rocksDB_directories_to_be_merged> <destinatnion_folder>
 *
 */

#include <fstream>
#include <cstdio>
#include <iostream>
#include <string>

#include "rocksdb/db.h"
#include "rocksdb/slice.h"
#include "rocksdb/options.h"

#include <experimental/filesystem>

using namespace rocksdb;
namespace fs = std::experimental::filesystem;

int main(int argc, char* argv[]) {
	if(argc != 3) {
		std::cerr << "Wrong number of arguments\nusage : ./<executable> <folder_having_rocksDB_directories_to_be_merged> <destinatnion_folder>\n";
		return 1;
	}
	std::string kDBPath = argv[2];
	DB* db;
  	Options options;
    	// Optimize RocksDB. This is the easiest way to get RocksDB to perform well
    	options.IncreaseParallelism();
    	options.OptimizeLevelStyleCompaction();
    	// create the DB if it's not already present
	options.create_if_missing = true;
             
    	// open DB
 	Status s = DB::Open(options, kDBPath, &db);
  	assert(s.ok());

	for (auto& dirEntry : fs::directory_iterator(argv[1])) {
		std::cout << dirEntry << "\n" ;
	
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
			std::cout << it->key().ToString() << ": " << it->value().ToString() << std::endl;
			s = db->Put(WriteOptions(), it->key().ToString(), it->value().ToString());
			assert(s.ok());
		}
	    	assert(it->status().ok()); // Check for any errors found during the scan
	  	delete it;
		delete cur_db;
	}
/*
	rocksdb::Iterator* it = db->NewIterator(rocksdb::ReadOptions());	
	for (it->SeekToFirst(); it->Valid(); it->Next()) {
	         std::cout << it->key().ToString() << ": " << it->value().ToString() << std::endl;
	}
	assert(it->status().ok()); // Check for any errors found during the scan
	delete it;
*/
	delete db;	
	return 0;
}
