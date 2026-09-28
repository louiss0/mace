package main

/*
#include <stdint.h>
*/
import "C"

//export trivial_answer
func trivial_answer() C.uint64_t {
	return 42
}

func main() {}
