// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Filipe Johansson

package utils

import (
	"fmt"
	"runtime/debug"
)

func SafeGoroutine(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("PANIC RECOVERED in %s: %v\nStack trace:\n%s\n",
					name, r, string(debug.Stack()))
			}
		}()
		fn()
	}()
}
