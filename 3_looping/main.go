package main

import (
	"fmt"
)


func main () {

	// for syntax is same as c++, we just not use brackets.
	for i:=1; i<10; i++ {
		fmt.Println(i)
	}

	
	// range x means 0 to (x-1)
	for i := range(7) {
		fmt.Print(i, " ")
	}


	// infinite loop
	for {
		fmt.Print("hi")
	}
}

// go dont have while loop but we can create one using for