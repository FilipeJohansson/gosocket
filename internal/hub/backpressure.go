// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Filipe Johansson

package hub

type BackpressurePolicy int

const (
	DropNewest BackpressurePolicy = iota
	DropOldest
	Block
)
