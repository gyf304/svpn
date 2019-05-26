package main

import (
	"github.com/songgao/water"
)

func getPlatformWaterConfig() water.Config {
	return water.Config{
		DeviceType: water.TAP,
		PlatformSpecificParams: water.PlatformSpecificParams{
		},
	}
}
