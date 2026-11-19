// Copyright (c) 2026 Cmacle. All rights reserved. See LICENSE.

package client

// oldDefaults holds the SHA-256 of every earlier version of the default art
// files that has changed. A file in the assets folder that matches one was
// copied out by an older version and never edited, so it can be replaced
// with the current default.
var oldDefaults = map[string][]string{
	"frames.png": {
		"954443f28a15b18cf97614bf700a2ee0a31cb2d2d00bac230fdf7f9e1cb4bf73",
		"e4cca71701337ed1520e31059734cd44d86ffa8eb55b83405a6652f19de502c1",
		"e8eececb1cedda5b688c360a90467e62018fe0e2b971c3d88c7ba19e653fa752",
	},
	"hats.png": {
		"2aae3b08d9ce2d103c51379e645daca7ab0d01ff71d9f0eaed758560a31969ee",
		"4b9fc5673d58007badc9e065ce07ea2b8e013dbdde725f51e1ef252553c5f3ed",
		"784ce475344ba7fd7d3168bbcc1b35c2b2c7523663d04fcf25e0da9264871ef6",
		"db4558eaf4376986a5e42cc59a0cf46e502c5800cb7f93d83e932d55e2e22a5e",
		"f4fbdc1a520700fecfd6a2ad79cd2052e8f8afc0a0110fedda32db62ad27794f",
	},
}
