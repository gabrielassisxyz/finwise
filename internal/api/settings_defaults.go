package api

// What the settings hold before the person changes anything is stated by the
// app's database alone, whose reads answer it where nothing is stored: the
// rules that classify new transactions read the same values below the API.

// The calibration bands tile the confidence range the threshold can take: five
// bands of ten points from the lowest threshold, the last one reaching 100.
const (
	calibrationFirstBand = 50
	calibrationBandWidth = 10
	calibrationBandCount = 5
	calibrationTop       = 100
)
