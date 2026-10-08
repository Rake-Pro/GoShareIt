//go:build !windows && !(linux && cgo)

package main

import "image"

// regionFreeze is unset on platforms whose overlay can show the live desktop.
var regionFreeze func() (image.Image, error)
