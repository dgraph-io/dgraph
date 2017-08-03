#!/bin/sh

# Generate Go assembly instructions for maxbits, pack, and unpack functions
python -m peachpy.x86_64 -mabi=goasm -S -o ../maxbits_amd64.s maxbits.py
python -m peachpy.x86_64 -mabi=goasm -S -o ../pack_amd64.s pack.py
python -m peachpy.x86_64 -mabi=goasm -S -o ../unpack_amd64.s unpack.py
