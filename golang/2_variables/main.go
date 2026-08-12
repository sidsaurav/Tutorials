package main

import "fmt"

func main() {

	// Three ways to define variables. 2 and 3 do type inference

	var name1 string = "Sid"
	var name2 = "Sid"
	name3 := "Sid"

	fmt.Println(name1, name2, name3)

	var num float32 // takes zero value by default
	num = 2.2
	fmt.Println(num)

	// we can't have unused variable

	const age = 30
}

/*
	1. we can use shorthand (:=) outside main func.
	2. we can have const without using it
*/

/*
	Zero Values -
	
	int -> 0
	float32 -> 0.0
	bool -> false
	string -> ""
*/