package main

import "fmt"

func main() {

	var names []string // zero value is nil
	fmt.Println(names)

	//create and initialize a slice
	num := []int{3}
	fmt.Println(num) // 3

	num = append(num, 2)
	fmt.Println(num) // 3, 2

	//using make func
	marks := make([]int, 5, 7) // make(type, number of elements to initilise with, capacity)
	fmt.Println(marks)


	// slice operator
	var arr = []int{12,14,75,13,88}
	slicedArr := arr[2:]
	fmt.Println(slicedArr) // [75,13,88]

	slicedArr[0] = 50 // this will make arr[2] as 50
	fmt.Println(arr, slicedArr)
}


// dynamic type arrays + more useful methods
// capacity is maximum elements it can hold without resizing the slice container
// the sliced array is shallow copy of original array which means they share memory.
// slice also work with strings
// to avoid changing original array we could use copy() for deep clone