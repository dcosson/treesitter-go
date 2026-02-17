package main

import "fmt"

func valid1() {
	fmt.Println("ok")
}

func broken(x {
	return
}

func valid2() int {
	return 42
}
