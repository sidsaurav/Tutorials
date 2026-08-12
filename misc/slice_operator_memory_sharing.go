// package main

// import (
// 	"fmt"
// 	"unsafe"
// )

// func isAddressSame(p1 interface{}, p2 interface{}) string {
// 	if p1 == p2 {
// 		return "same address"
// 	}
// 	return "different address"
// }

// func main () {
// 	var arr = []int{1,2,3,4,5}
// 	var arr2 = arr[:3]
// 	fmt.Println(arr, arr2, isAddressSame(unsafe.Pointer(&arr[0]), unsafe.Pointer(&arr2[0])))

// 	arr2 = append(arr2, 11)
// 	fmt.Println(arr, arr2, isAddressSame(unsafe.Pointer(&arr[0]), unsafe.Pointer(&arr2[0])))
	
// 	arr2 = append(arr2, 22)
// 	fmt.Println(arr, arr2, isAddressSame(unsafe.Pointer(&arr[0]), unsafe.Pointer(&arr2[0])))

// 	arr2 = append(arr2, 33)
// 	fmt.Println(arr, arr2, isAddressSame(unsafe.Pointer(&arr[0]), unsafe.Pointer(&arr2[0])))
// }

// /*
// 	Output
	
// 	[1 2 3 4 5] [1 2 3] same address
// 	[1 2 3 11 5] [1 2 3 11] same address
// 	[1 2 3 11 22] [1 2 3 11 22] same address
// 	[1 2 3 11 22] [1 2 3 11 22 33] different address
// */

// // to stop chaning the original array we could use arr[x:y:y] 2nd arg == 3rd arg so cap is restricted to 3rd arg
// // or we could use copy() to create a deep clone