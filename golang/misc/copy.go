package main

import "fmt"

func main () {

	var slice = []string{"a","b","c","d","e"}

	//deep copy method 1
	var deepCopy1 = append([]string{}, slice...)
	fmt.Println(deepCopy1, &slice[0], &deepCopy1[0])

	//deep copy method 2 -- using copy()
	var deepCopy2 = make([]string, len(slice))
	copy(deepCopy2, slice)
	fmt.Println(deepCopy1, &slice[0], &deepCopy2[0])

	// we can do many operations using copy() and slice operator like
	// shift, unshift, delete/add element from mid

	//ex1 - shift
	var arr = []string{"a","b","c","d"}
	temp := arr[len(arr) - 1]
	copy(arr[1:], arr)
	arr[0] = temp
	fmt.Println(arr) // [d a b c]

	//ex2 - delete 2nd element
	var arr2 = []string{"a","b","c","d","e"}
	copy(arr2[2:], arr2[3:])
	arr2 = arr2[:len(arr2) - 1]
	fmt.Println(arr2) // [a b d e]


}

/*
	n := copy(dest, src)
	here n is number of elements copied which could be min(len(dest), len(src))

	copy() do deep copy unless it is multidimensional slice.
	in such case we need to interate over every element of outer array and do copy() on inner array
*/