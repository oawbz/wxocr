package ocr

import "math"

// cropTransform solves the source-to-crop homography, rounds its coefficients
// to native float32, and inverts it for OpenCV-style destination sampling.
func cropTransform(q quadrilateral, w, h int) [9]float64 {
	dst := quadrilateral{{0, 0}, {float64(w), 0}, {float64(w), float64(h)}, {0, float64(h)}}
	var a [8][9]float64
	for i, p := range q {
		x, y := float64(float32(p.x)), float64(float32(p.y))
		u, v := dst[i].x, dst[i].y
		a[2*i] = [9]float64{x, y, 1, 0, 0, 0, -u * x, -u * y, u}
		a[2*i+1] = [9]float64{0, 0, 0, x, y, 1, -v * x, -v * y, v}
	}
	for k := 0; k < 8; k++ {
		pivot := k
		for i := k + 1; i < 8; i++ {
			if math.Abs(a[i][k]) > math.Abs(a[pivot][k]) {
				pivot = i
			}
		}
		a[k], a[pivot] = a[pivot], a[k]
		divisor := a[k][k]
		if divisor == 0 {
			return [9]float64{0, 0, q[0].x, 0, 0, q[0].y, 0, 0, 1}
		}
		for j := k; j < 9; j++ {
			a[k][j] /= divisor
		}
		for i := 0; i < 8; i++ {
			if i != k {
				f := a[i][k]
				for j := k; j < 9; j++ {
					a[i][j] -= f * a[k][j]
				}
			}
		}
	}
	var m [9]float64
	for i := 0; i < 8; i++ {
		m[i] = float64(float32(a[i][8]))
	}
	m[8] = 1
	inv := [9]float64{m[4]*m[8] - m[5]*m[7], m[2]*m[7] - m[1]*m[8], m[1]*m[5] - m[2]*m[4], m[5]*m[6] - m[3]*m[8], m[0]*m[8] - m[2]*m[6], m[2]*m[3] - m[0]*m[5], m[3]*m[7] - m[4]*m[6], m[1]*m[6] - m[0]*m[7], m[0]*m[4] - m[1]*m[3]}
	return inv // Homogeneous division cancels the common determinant.
}
