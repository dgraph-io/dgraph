//  Copyright (c) 2011-present, Facebook, Inc.  All rights reserved.
//  This source code is licensed under the BSD-style license found in the
//  LICENSE file in the root directory of this source tree. An additional grant
//  of patent rights can be found in the PATENTS file in the same directory.
//
// Copyright (c) 2011 The LevelDB Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file. See the AUTHORS file for names of contributors.
#pragma once

#include <stdint.h>
#include <mutex>
#include <string>

#include "rocksdb/Status.h"
#include "rocksdb/env.h"
#include "util/aligned_buffer.h"

#include <windows.h>


namespace rocksdb {
namespace port {

std::string GetWindowsErrSz(DWORD err);

inline Status IOErrorFromWindowsError(const std::string& context, DWORD err) {
  return ((err == ERROR_HANDLE_DISK_FULL) || (err == ERROR_DISK_FULL))
             ? Status::NoSpace(context, GetWindowsErrSz(err))
             : Status::IOError(context, GetWindowsErrSz(err));
}

inline Status IOErrorFromLastWindowsError(const std::string& context) {
  return IOErrorFromWindowsError(context, GetLastError());
}

inline Status IOError(const std::string& context, int err_number) {
  return (err_number == ENOSPC)
             ? Status::NoSpace(context, strerror(err_number))
             : Status::IOError(context, strerror(err_number));
}

// Note the below two do not set errno because they are used only here in this
// file
// on a Windows handle and, therefore, not necessary. Translating GetLastError()
// to errno
// is a sad business
inline int fsync(HANDLE hFile) {
  if (!FlushFileBuffers(hFile)) {
    return -1;
  }

  return 0;
}

SSIZE_T pwrite(HANDLE hFile, const char* src, size_t numBytes, uint64_t offset);

SSIZE_T pread(HANDLE hFile, char* src, size_t numBytes, uint64_t offset);

Status fallocate(const std::string& filename, HANDLE hFile, uint64_t to_size);

Status ftruncate(const std::string& filename, HANDLE hFile, uint64_t toSize);

size_t GetUniqueIdFromFile(HANDLE hFile, char* id, size_t max_size);

class WinFileData {
 protected:
  const std::string filename_;
  HANDLE hFile_;
  // If ture,  the I/O issued would be direct I/O which the buffer
  // will need to be aligned (not sure there is a guarantee that the buffer
  // passed in is aligned).
  const bool use_direct_io_;

 public:
  // We want this class be usable both for inheritance (prive
  // or protected) and for containment so __ctor and __dtor public
  WinFileData(const std::string& filename, HANDLE hFile, bool use_direct_io)
      : filename_(filename), hFile_(hFile), use_direct_io_(use_direct_io) {}

  virtual ~WinFileData() { this->CloseFile(); }

  bool CloseFile() {
    bool result = true;

    if (hFile_ != NULL && hFile_ != INVALID_HANDLE_VALUE) {
      result = ::CloseHandle(hFile_);
      assert(result);
      hFile_ = NULL;
    }
    return result;
  }

  const std::string& GetName() const { return filename_; }

  HANDLE GetFileHandle() const { return hFile_; }

  bool use_direct_io() const { return use_direct_io_; }

  WinFileData(const WinFileData&) = delete;
  WinFileData& operator=(const WinFileData&) = delete;
};

// mmap() based random-access
class WinMmapReadableFile : private WinFileData, public RandomAccessFile {
  HANDLE hMap_;

  const void* mapped_region_;
  const size_t length_;

 public:
  // mapped_region_[0,length-1] contains the mmapped contents of the file.
  WinMmapReadableFile(const std::string& fileName, HANDLE hFile, HANDLE hMap,
                      const void* mapped_region, size_t length);

  ~WinMmapReadableFile();

  WinMmapReadableFile(const WinMmapReadableFile&) = delete;
  WinMmapReadableFile& operator=(const WinMmapReadableFile&) = delete;

  virtual Status Read(uint64_t offset, size_t n, Slice* result,
                      char* scratch) const override;

  virtual Status InvalidateCache(size_t offset, size_t length) override;

  virtual size_t GetUniqueId(char* id, size_t max_size) const override;
};

// We preallocate and use memcpy to append new
// data to the file.  This is safe since we either properly close the
// file before reading from it, or for log files, the reading code
// knows enough to skip zero suffixes.
class WinMmapFile : private WinFileData, public WritableFile {
 private:
  HANDLE hMap_;

  const size_t page_size_;  // We flush the mapping view in page_size
  // increments. We may decide if this is a memory
  // page size or SSD page size
  const size_t
      allocation_granularity_;  // View must start at such a granularity

  size_t reserved_size_;  // Preallocated size

  size_t mapping_size_;  // The max size of the mapping object
  // we want to guess the final file size to minimize the remapping
  size_t view_size_;  // How much memory to map into a view at a time

  char* mapped_begin_;  // Must begin at the file offset that is aligned with
  // allocation_granularity_
  char* mapped_end_;
  char* dst_;  // Where to write next  (in range [mapped_begin_,mapped_end_])
  char* last_sync_;  // Where have we synced up to

  uint64_t file_offset_;  // Offset of mapped_begin_ in file

  // Do we have unsynced writes?
  bool pending_sync_;

  // Can only truncate or reserve to a sector size aligned if
  // used on files that are opened with Unbuffered I/O
  Status TruncateFile(uint64_t toSize);

  Status UnmapCurrentRegion();

  Status MapNewRegion();

  virtual Status PreallocateInternal(uint64_t spaceToReserve);

 public:
  WinMmapFile(const std::string& fname, HANDLE hFile, size_t page_size,
              size_t allocation_granularity, const EnvOptions& options);

  ~WinMmapFile();

  WinMmapFile(const WinMmapFile&) = delete;
  WinMmapFile& operator=(const WinMmapFile&) = delete;

  virtual Status Append(const Slice& data) override;

  // Means Close() will properly take care of truncate
  // and it does not need any additional information
  virtual Status Truncate(uint64_t size) override;

  virtual Status Close() override;

  virtual Status Flush() override;

  // Flush only data
  virtual Status Sync() override;

  /**
  * Flush data as well as metadata to stable storage.
  */
  virtual Status Fsync() override;

  /**
  * Get the size of valid data in the file. This will not match the
  * size that is returned from the filesystem because we use mmap
  * to extend file by map_size every time.
  */
  virtual uint64_t GetFileSize() override;

  virtual Status InvalidateCache(size_t offset, size_t length) override;

  virtual Status Allocate(uint64_t offset, uint64_t len) override;

  virtual size_t GetUniqueId(char* id, size_t max_size) const override;
};

class WinSequentialFile : private WinFileData, public SequentialFile {
 public:
  WinSequentialFile(const std::string& fname, HANDLE f,
                    const EnvOptions& options);

  ~WinSequentialFile();

  WinSequentialFile(const WinSequentialFile&) = delete;
  WinSequentialFile& operator=(const WinSequentialFile&) = delete;

  virtual Status Read(size_t n, Slice* result, char* scratch) override;

  virtual Status Skip(uint64_t n) override;

  virtual Status InvalidateCache(size_t offset, size_t length) override;
};

class WinRandomAccessImpl {
 protected:
  WinFileData* file_base_;
  bool read_ahead_;
  const size_t compaction_readahead_size_;
  const size_t random_access_max_buffer_size_;
  mutable std::mutex buffer_mut_;
  mutable AlignedBuffer buffer_;
  mutable uint64_t
      buffered_start_;  // file offset set that is currently buffered

  // Override for behavior change when creating a custom env
  virtual SSIZE_T PositionedReadInternal(char* src, size_t numBytes,
                                         uint64_t offset) const;

  /*
  * The function reads a requested amount of bytes into the specified aligned
  * buffer Upon success the function sets the length of the buffer to the
  * amount of bytes actually read even though it might be less than actually
  * requested. It then copies the amount of bytes requested by the user (left)
  * to the user supplied buffer (dest) and reduces left by the amount of bytes
  * copied to the user buffer
  *
  * @user_offset [in] - offset on disk where the read was requested by the user
  * @first_page_start [in] - actual page aligned disk offset that we want to
  *                          read from
  * @bytes_to_read [in] - total amount of bytes that will be read from disk
  *                       which is generally greater or equal to the amount
  *                       that the user has requested due to the
  *                       either alignment requirements or read_ahead in
  *                       effect.
  * @left [in/out] total amount of bytes that needs to be copied to the user
  *                buffer. It is reduced by the amount of bytes that actually
  *                copied
  * @buffer - buffer to use
  * @dest - user supplied buffer
  */

  SSIZE_T ReadIntoBuffer(uint64_t user_offset, uint64_t first_page_start,
                         size_t bytes_to_read, size_t& left,
                         AlignedBuffer& buffer, char* dest) const;

  SSIZE_T ReadIntoOneShotBuffer(uint64_t user_offset, uint64_t first_page_start,
                                size_t bytes_to_read, size_t& left,
                                char* dest) const;

  SSIZE_T ReadIntoInstanceBuffer(uint64_t user_offset,
                                 uint64_t first_page_start,
                                 size_t bytes_to_read, size_t& left,
                                 char* dest) const;

  WinRandomAccessImpl(WinFileData* file_base, size_t alignment,
                      const EnvOptions& options);

  virtual ~WinRandomAccessImpl() {}

 public:
  WinRandomAccessImpl(const WinRandomAccessImpl&) = delete;
  WinRandomAccessImpl& operator=(const WinRandomAccessImpl&) = delete;

  Status ReadImpl(uint64_t offset, size_t n, Slice* result,
                  char* scratch) const;

  void HintImpl(RandomAccessFile::AccessPattern pattern);
};

// pread() based random-access
class WinRandomAccessFile
    : private WinFileData,
      protected WinRandomAccessImpl,  // Want to be able to override
                                      // PositionedReadInternal
      public RandomAccessFile {
 public:
  WinRandomAccessFile(const std::string& fname, HANDLE hFile, size_t alignment,
                      const EnvOptions& options);

  ~WinRandomAccessFile();

  virtual void EnableReadAhead() override;

  virtual Status Read(uint64_t offset, size_t n, Slice* result,
                      char* scratch) const override;

  virtual bool ShouldForwardRawRequest() const override;

  virtual void Hint(AccessPattern pattern) override;

  virtual Status InvalidateCache(size_t offset, size_t length) override;

  virtual size_t GetUniqueId(char* id, size_t max_size) const override;
};

// This is a sequential write class. It has been mimicked (as others) after
// the original Posix class. We add support for unbuffered I/O on windows as
// well
// we utilize the original buffer as an alignment buffer to write directly to
// file with no buffering.
// No buffering requires that the provided buffer is aligned to the physical
// sector size (SSD page size) and
// that all SetFilePointer() operations to occur with such an alignment.
// We thus always write in sector/page size increments to the drive and leave
// the tail for the next write OR for Close() at which point we pad with zeros.
// No padding is required for
// buffered access.
class WinWritableImpl {
 protected:
  WinFileData* file_data_;
  const uint64_t alignment_;
  uint64_t filesize_;      // How much data is actually written disk
  uint64_t reservedsize_;  // how far we have reserved space

  virtual Status PreallocateInternal(uint64_t spaceToReserve);

  WinWritableImpl(WinFileData* file_data, size_t alignment);

  ~WinWritableImpl() {}

  uint64_t GetAlignement() const { return alignment_; }

  Status AppendImpl(const Slice& data);

  // Requires that the data is aligned as specified by
  // GetRequiredBufferAlignment()
  Status PositionedAppendImpl(const Slice& data, uint64_t offset);

  Status TruncateImpl(uint64_t size);

  Status CloseImpl();

  Status SyncImpl();

  uint64_t GetFileSizeImpl() {
    // Double accounting now here with WritableFileWriter
    // and this size will be wrong when unbuffered access is used
    // but tests implement their own writable files and do not use
    // WritableFileWrapper
    // so we need to squeeze a square peg through
    // a round hole here.
    return filesize_;
  }

  Status AllocateImpl(uint64_t offset, uint64_t len);

 public:
  WinWritableImpl(const WinWritableImpl&) = delete;
  WinWritableImpl& operator=(const WinWritableImpl&) = delete;
};

class WinWritableFile : private WinFileData,
                        protected WinWritableImpl,
                        public WritableFile {
 public:
  WinWritableFile(const std::string& fname, HANDLE hFile, size_t alignment,
                  size_t capacity, const EnvOptions& options);

  ~WinWritableFile();

  // Indicates if the class makes use of direct I/O
  // Use PositionedAppend
  virtual bool use_direct_io() const override;

  virtual size_t GetRequiredBufferAlignment() const override;

  virtual Status Append(const Slice& data) override;

  // Requires that the data is aligned as specified by
  // GetRequiredBufferAlignment()
  virtual Status PositionedAppend(const Slice& data, uint64_t offset) override;

  // Need to implement this so the file is truncated correctly
  // when buffered and unbuffered mode
  virtual Status Truncate(uint64_t size) override;

  virtual Status Close() override;

  // write out the cached data to the OS cache
  // This is now taken care of the WritableFileWriter
  virtual Status Flush() override;

  virtual Status Sync() override;

  virtual Status Fsync() override;

  virtual uint64_t GetFileSize() override;

  virtual Status Allocate(uint64_t offset, uint64_t len) override;

  virtual size_t GetUniqueId(char* id, size_t max_size) const override;
};

class WinRandomRWFile : private WinFileData,
                        protected WinRandomAccessImpl,
                        protected WinWritableImpl,
                        public RandomRWFile {
 public:
  WinRandomRWFile(const std::string& fname, HANDLE hFile, size_t alignment,
                  const EnvOptions& options);

  ~WinRandomRWFile() {}

  // Indicates if the class makes use of direct I/O
  // If false you must pass aligned buffer to Write()
  virtual bool use_direct_io() const override;

  // Use the returned alignment value to allocate aligned
  // buffer for Write() when use_direct_io() returns true
  virtual size_t GetRequiredBufferAlignment() const override;

  // Used by the file_reader_writer to decide if the ReadAhead wrapper
  // should simply forward the call and do not enact read_ahead buffering or
  // locking.
  // The implementation below takes care of reading ahead
  virtual bool ShouldForwardRawRequest() const override;

  // For cases when read-ahead is implemented in the platform dependent
  // layer. This is when ShouldForwardRawRequest() returns true.
  virtual void EnableReadAhead() override;

  // Write bytes in `data` at  offset `offset`, Returns Status::OK() on success.
  // Pass aligned buffer when use_direct_io() returns true.
  virtual Status Write(uint64_t offset, const Slice& data) override;

  // Read up to `n` bytes starting from offset `offset` and store them in
  // result, provided `scratch` size should be at least `n`.
  // Returns Status::OK() on success.
  virtual Status Read(uint64_t offset, size_t n, Slice* result,
                      char* scratch) const override;

  virtual Status Flush() override;

  virtual Status Sync() override;

  virtual Status Fsync() { return Sync(); }

  virtual Status Close() override;
};

class WinDirectory : public Directory {
 public:
  WinDirectory() {}

  virtual Status Fsync() override;
};

class WinFileLock : public FileLock {
 public:
  explicit WinFileLock(HANDLE hFile) : hFile_(hFile) {
    assert(hFile != NULL);
    assert(hFile != INVALID_HANDLE_VALUE);
  }

  ~WinFileLock();

 private:
  HANDLE hFile_;
};
}
}
