#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

#include <unicode/ustring.h>
#include <unicode/ubrk.h>

typedef struct Tokenizer Tokenizer;

Tokenizer* NewTokenizer(const char* input, int len, UErrorCode* err);

void DestroyTokenizer(Tokenizer* tokenizer);

char* TokenizerNext(Tokenizer* tokenizer);

int TokenizerDone(Tokenizer* tokenizer);