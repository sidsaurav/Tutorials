package main

import "fmt"

func main() {
	var nums [5]int
	nums[1] = 344

	fmt.Println(len(nums)) // length of arr
	fmt.Println(nums[1]) // get element at an index
	fmt.Println(nums) // print whole array

	//declare and define in same line --- type{initialising values}
	students := [3]string{"a", "b", "c"}
	fmt.Println(students)


	//2d array
	plane := [2][3]int {{1,2,3}, {4,5,6}}
	fmt.Print(plane)
}

// number and type of elements defined
