GO ?= go

BUILDMODE := install

.PHONY: build 
build: BUILDMODE = build
build: install
	mkdir dgraph;\
	cd dgraph;\
	cp ../cmd/dgraph/dgraph .;\
	cp ../cmd/dgraphassigner/dgraphassigner .;\
	cp ../cmd/dgraphloader/dgraphloader .;\
	cp ../cmd/dgraphlist/dgraphlist .;\
	cp ../cmd/dgraphmerge/dgraphmerge .;\
	cd ..;\
	tar -zcf dgraph-linux-amd64-v0.4.0.tar.gz -C dgraph .;\
	rm -rf dgraph;\

.PHONY: install
install:	dgraph	dgraphloader	dgraphassigner	dgraphlist	dgraphmerge

.PHONY: dgraph
dgraph:
	cd "./cmd/dgraph" ; \
	$(GO) $(BUILDMODE) --tags=embed . ; 

.PHONY: dgraphassigner
dgraphassigner:
	cd "./cmd/dgraphassigner" ; \
		$(GO) $(BUILDMODE) --tags=embed . ;

.PHONY: dgraphloader
dgraphloader:
	cd "./cmd/dgraphloader" ; \
		$(GO) $(BUILDMODE) --tags=embed . ;

.PHONY: dgraphlist
dgraphlist:
	cd "./cmd/dgraphlist" ; \
		$(GO) $(BUILDMODE) --tags=embed . ;

.PHONY: dgraphmerge
dgraphmerge:
	cd "./cmd/dgraphmerge" ; \
		$(GO) $(BUILDMODE) --tags=embed . ;

.PHONY: test
test:
	go test ./...
