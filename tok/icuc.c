#include "icuc.h"

#define kMaxTokenSize 1000

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
// tokenizer->token. However, if we run out of tokens, we return a nullptr.
char* TokenizerNext(Tokenizer* tokenizer) {
	const int start = tokenizer->end;
	if (start >= tokenizer->len) {
		return 0;
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
	return tokenizer->token;
}

// TokenizerDone returns whether the tokenizer is out of tokens.
int TokenizerDone(Tokenizer* tokenizer) {
	return tokenizer->end == UBRK_DONE;
}
