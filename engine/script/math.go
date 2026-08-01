package script

import "math"

// mathFloat32frombits is a local alias for math.Float32frombits kept in
// its own file so the rest of the package avoids importing math.
func mathFloat32frombits(b uint32) float32 {
	return math.Float32frombits(b)
}
