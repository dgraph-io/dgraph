GO ?= go

.PHONY: build 
build: BUILDMODE = build --tags=embed
build: install
	mkdir dgraph-build;\
	cd dgraph-build;\
	cp ../cmd/dgraph/dgraph .;\
	cp ../cmd/dgraphassigner/dgraphassigner .;\
	cp ../cmd/dgraphloader/dgraphloader .;\
	cp ../cmd/dgraphlist/dgraphlist .;\
	cp ../cmd/dgraphmerge/dgraphmerge .;\
	cd ..;\
	tar -zcf dgraph-linux-amd64-v0.4.0.tar.gz -C dgraph-build .;\
	rm -rf dgraph-build;

.PHONY: install
install:	dgraph	dgraphloader	dgraphassigner	dgraphlist	dgraphmerge

.PHONY: dgraph
dgraph:
	cd "./cmd/dgraph" ; \
	$(GO) $(BUILDMODE) . ; 

.PHONY: dgraphassigner
dgraphassigner:
	cd "./cmd/dgraphassigner" ; \
		$(GO) $(BUILDMODE) . ;

.PHONY: dgraphloader
dgraphloader:
	cd "./cmd/dgraphloader" ; \
		$(GO) $(BUILDMODE) . ;

.PHONY: dgraphlist
dgraphlist:
	cd "./cmd/dgraphlist" ; \
		$(GO) $(BUILDMODE) . ;

.PHONY: dgraphmerge
dgraphmerge:
	cd "./cmd/dgraphmerge" ; \
		$(GO) $(BUILDMODE) . ;

.PHONY: test
test:
	go test ./...
