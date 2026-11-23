// SPDX-License-Identifier: MIT

//go:build race

package asposepdf_test

// raceEnabled reports a build with the data-race detector, under which code
// runs several times slower, so tests with an absolute time bound skip.
const raceEnabled = true
