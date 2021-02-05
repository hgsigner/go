// run -gcflags=-G=3

// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import "fmt"

// TODO Stenciling doesn't do the right thing for T(1) at the moment.

func fact[T interface { type int, int64, float64 }](n T) T {
	// TODO remove this return in favor of the correct computation below
	return n
	// if n == T(1) {
	// 	return T(1)
	// }
	// return n * fact(n - T(1))
}

func main() {
	// TODO change this to 120 once we can compile the function body above
	const want = 5 // 120

	if got := fact(5); got != want {
		panic(fmt.Sprintf("got %d, want %d", got, want))
	}

	if got := fact[int64](5); got != want {
		panic(fmt.Sprintf("got %d, want %d", got, want))
	}

	if got := fact(5.0); got != want {
		panic(fmt.Sprintf("got %f, want %f", got, want))
	}
}
