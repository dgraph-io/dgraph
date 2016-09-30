/*
 * Copyright 2016 Dgraph Labs, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 		http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#ifdef ENABLE_ICU

#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

#include <unicode/ustring.h>
#include <unicode/ubrk.h>

typedef struct Tokenizer Tokenizer;

Tokenizer* NewTokenizer(const char* input, int len, int max_token_size,
  UErrorCode* err);

void DestroyTokenizer(Tokenizer* tokenizer);

int TokenizerNext(Tokenizer* tokenizer);

char* TokenizerToken(Tokenizer* tokenizer);

#endif