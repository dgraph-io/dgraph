package main

import (
	"bytes"
	"fmt"
)

type Context struct{
	count int
	positionOfStart int
	startPositionOfProperty int
	header map[int]string
	text bytes.Buffer
	rdfLines bytes.Buffer
}

// This function processes the first line in the file
func (context *Context) processHeader(headerLine string){
	fmt.Println("processing header")
	fmt.Print(headerLine )
	fmt.Println("Finished processing header")
}


// This function processes second line onwards in the file
func (context *Context) processLineItem(lineItem string){
	fmt.Println("processing line item")
	fmt.Print(lineItem )
	fmt.Println("Finished processing line item")
}
