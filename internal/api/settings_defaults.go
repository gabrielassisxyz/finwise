package api

// What the settings hold before the person changes anything. The backend has
// nowhere to keep a change yet, so these are also what every read answers. They
// are the values the stand-in starts with, and nothing else in the package
// restates them.
const (
	defaultConfidenceThreshold = 85
	defaultMinExamples         = 5
	defaultManualClosingDay    = 28
	defaultClosingMode         = "auto"
)

// The calibration bands tile the confidence range the threshold can take: five
// bands of ten points from the lowest threshold, the last one reaching 100.
const (
	calibrationFirstBand = 50
	calibrationBandWidth = 10
	calibrationBandCount = 5
	calibrationTop       = 100
)
