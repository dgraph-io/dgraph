package main

type Source struct{
	count int
}

// function to increment count in Source
func (source *Source) increment(){
	source.count=source.count+1
}
func (source *Source) get() int{
	return source.count*10
}
