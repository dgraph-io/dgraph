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

#include "icuc.h"

#define kMaxTokenSize 100

struct Tokenizer {
	UChar* buf;  // Our input string converted to UChar array. We own this array.
	UBreakIterator* iter;
	int len;  // Number of UChar's.
	int end;  // Index into buf. It tells us where the last token ends.
	char token[kMaxTokenSize];  // For storing results of TokenizerNext.
};

// NewTokenizer creates new Tokenizer object given some input string.
// CAUTION: input string should be null-terminated.
// convert the input to an array of UChar where each UChar takes 2 bytes.
Tokenizer* NewTokenizer(const char* input, int len, UErrorCode* err) {
	Tokenizer* tokenizer = (Tokenizer*)malloc(sizeof(Tokenizer));
	tokenizer->buf = (UChar*)malloc(sizeof(UChar) * (len + 5));
	tokenizer->token[0] = 0;
	
	// Convert char array to UChar array.
	u_uastrcpy(tokenizer->buf, input);
	
	// Somehow, iterating until we see UBRK_END doesn't work so great. Let's just
	// determine the number of runes at the outset.
	tokenizer->len = u_strlen(tokenizer->buf);
	
	// Prepares our iterator object.
	tokenizer->iter = ubrk_open(UBRK_WORD, "", tokenizer->buf, len, err);
	tokenizer->end = ubrk_first(tokenizer->iter);
	return tokenizer;
}

// DestroyTokenizer frees Tokenizer object.
void DestroyTokenizer(Tokenizer* tokenizer) {
	ubrk_close(tokenizer->iter);
	free(tokenizer->buf);
	free(tokenizer);
}

// TokenizerNext copies the next token into tokenizer->token and returns
// number of chars written. However, if we run out of tokens, we return -1.
int TokenizerNext(Tokenizer* tokenizer) {
	const int start = tokenizer->end;
	if (start >= tokenizer->len) {
		tokenizer->token[0] = 0;
		return -1;
	}
	const int end = ubrk_next(tokenizer->iter);
  tokenizer->end = end;
	
	// Put a zero in UChar array. But before that, do some backup.
	const UChar backup = tokenizer->buf[end];
	tokenizer->buf[end] = 0;
	
	// Want to copy tokenizer->end to new_end.
	u_austrncpy(tokenizer->token, tokenizer->buf + start, kMaxTokenSize - 1);
	tokenizer->token[kMaxTokenSize - 1] = 0;  // Just in case we hit token's limit.
	
	tokenizer->buf[end] = backup;
	// The strlen here seems expensive, but I prefer working with []byte in Go and
	// for that, we need the length and it is better to do it in C than in Go.
	// u_austrncpy does not seem to return any length information.
	// An alternative is to use the C++ API but the C++ code looks more convoluted
	// and might make the job for embedding more tricky.
	return strlen(tokenizer->token);
}

// TokenizerToken returns the last token written.
char* TokenizerToken(Tokenizer* tokenizer) {
	return tokenizer->token;
}

#endif