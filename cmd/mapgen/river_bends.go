package main

import (
	"image"
	"math"
)

type riverBendPoint struct {
	column, row, distance float64
}

func buildNestedBendPath(width, height, startColumn, startRow, endColumn, endRow int, seed int64, opts RiverOptions) []int {
	deltaColumn, deltaRow := float64(endColumn-startColumn), float64(endRow-startRow)
	distance := math.Hypot(deltaColumn, deltaRow)
	if distance < 1 {
		return []int{tileIndex(startColumn, startRow, width)}
	}
	wavelength := float64(opts.ShapeWavelengthTiles) / opts.ShapeFrequencyScale
	wavelength = maxFloat(32, wavelength)
	amplitude := maxFloat(float64(opts.RiverWidthMax*3), float64(minInt(width, height))*opts.MeanderStrength*8.5) * opts.ShapeAmplitudeScale
	amplitude = minFloat(amplitude, minFloat(distance*opts.ShapeDistanceCap, wavelength*0.30))
	controlSpacing := maxFloat(4, minFloat(float64(maxInt(1, opts.ShapeSegmentLength)), wavelength/8))
	segments := maxInt(2, int(math.Ceil(distance/controlSpacing)))
	controls := make([]riverBendPoint, segments+1)
	for index := range controls {
		progress := float64(index) / float64(segments)
		along := distance * progress
		blend := riverBendBlend(along, distance, wavelength)
		offset := amplitude * blend * riverBendNoise(seed, along, wavelength)
		alongOffset := amplitude * opts.ShapeAlongScale * 0.2 * blend * riverBendNoise(seed+113, along, wavelength/2)
		controls[index] = riverBendPoint{
			column: float64(startColumn) + deltaColumn*progress - deltaRow/distance*offset + deltaColumn/distance*alongOffset,
			row:    float64(startRow) + deltaRow*progress + deltaColumn/distance*offset + deltaRow/distance*alongOffset,
		}
	}
	broad := sampleRiverBendCurve(controls, 2)
	shortest := maxFloat(16, wavelength/math.Pow(2, float64(opts.ShapeOctaves-1)))
	points := resampleRiverBendCurve(broad, maxFloat(2, minFloat(controlSpacing, shortest/6)))
	if len(points) < 2 {
		return nil
	}
	length := points[len(points)-1].distance
	detailed := append([]riverBendPoint(nil), points...)
	for index := 1; index < len(points)-1; index++ {
		point := points[index]
		before, after := points[index-1], points[index+1]
		tangentColumn, tangentRow := after.column-before.column, after.row-before.row
		tangentLength := math.Hypot(tangentColumn, tangentRow)
		if tangentLength == 0 {
			continue
		}
		weight, detailWavelength, displacement := opts.ShapeOctaveGain, wavelength/2, 0.0
		for octave := 1; octave < opts.ShapeOctaves; octave++ {
			if detailWavelength < maxFloat(16, float64(opts.FairwayWidthTiles*4)) {
				break
			}
			displacement += weight * riverBendNoise(seed+int64(octave)*7919, point.distance, detailWavelength)
			weight *= opts.ShapeOctaveGain
			detailWavelength /= 2
		}
		calm := 0.45 + 0.55*smoothHashNoise2D(seed+107, point.distance, 0, wavelength*0.7, riverMeanderSalt)
		displacement *= amplitude * calm * riverBendBlend(point.distance, length, wavelength)
		detailed[index].column -= tangentRow / tangentLength * displacement
		detailed[index].row += tangentColumn / tangentLength * displacement
	}
	curve := sampleRiverBendCurve(detailed, 1)
	path := make([]int, 0, len(curve)*2)
	previousColumn, previousRow := startColumn, startRow
	path = append(path, tileIndex(previousColumn, previousRow, width))
	for _, point := range curve {
		column := clampInt(int(math.Round(point.column)), 0, width-1)
		row := clampInt(int(math.Round(point.row)), 0, height-1)
		path = appendCardinalRiverSegment(path, width, previousColumn, previousRow, column, row)
		previousColumn, previousRow = column, row
		if len(path) > 8*(width+height) {
			return nil
		}
	}
	return path
}

func riverBendNoise(seed int64, distance, wavelength float64) float64 {
	return 2*smoothHashNoise2D(seed, distance, 0, wavelength, riverMeanderSalt) - 1
}

func riverBendBlend(distance, length, wavelength float64) float64 {
	approach := maxFloat(1, minFloat(length/4, wavelength/5))
	weight := minFloat(1, minFloat(distance, length-distance)/approach)
	return weight * weight * (3 - 2*weight)
}

func sampleRiverBendCurve(controls []riverBendPoint, spacing float64) []riverBendPoint {
	if len(controls) < 2 {
		return controls
	}
	points := []riverBendPoint{controls[0]}
	for segment := 0; segment < len(controls)-1; segment++ {
		before, start := controls[maxInt(0, segment-1)], controls[segment]
		end, after := controls[segment+1], controls[minInt(len(controls)-1, segment+2)]
		steps := maxInt(1, int(math.Ceil(math.Hypot(end.column-start.column, end.row-start.row)/spacing)))
		for sample := 1; sample <= steps; sample++ {
			progress := float64(sample) / float64(steps)
			point := riverBendPoint{
				column: catmullRom(before.column, start.column, end.column, after.column, progress),
				row:    catmullRom(before.row, start.row, end.row, after.row, progress),
			}
			previous := points[len(points)-1]
			point.distance = previous.distance + math.Hypot(point.column-previous.column, point.row-previous.row)
			points = append(points, point)
		}
	}
	return points
}

func resampleRiverBendCurve(curve []riverBendPoint, spacing float64) []riverBendPoint {
	if len(curve) < 2 {
		return curve
	}
	points := []riverBendPoint{curve[0]}
	segment := 1
	for distance := spacing; distance < curve[len(curve)-1].distance; distance += spacing {
		for segment < len(curve)-1 && curve[segment].distance < distance {
			segment++
		}
		before, after := curve[segment-1], curve[segment]
		fraction := (distance - before.distance) / (after.distance - before.distance)
		points = append(points, riverBendPoint{
			column: before.column + fraction*(after.column-before.column),
			row:    before.row + fraction*(after.row-before.row), distance: distance,
		})
	}
	return append(points, curve[len(curve)-1])
}

func appendCardinalRiverSegment(path []int, width, startColumn, startRow, endColumn, endRow int) []int {
	deltaColumn, deltaRow := absInt(endColumn-startColumn), absInt(endRow-startRow)
	stepColumn, stepRow := signInt(endColumn-startColumn), signInt(endRow-startRow)
	errorTerm := deltaColumn - deltaRow
	appendPoint := func(column, row int) {
		index := tileIndex(column, row, width)
		if len(path) > 0 && path[len(path)-1] == index {
			return
		}
		if len(path) > 1 && path[len(path)-2] == index {
			path = path[:len(path)-1]
			return
		}
		path = append(path, index)
	}
	appendPoint(startColumn, startRow)
	for startColumn != endColumn || startRow != endRow {
		doubledError := 2 * errorTerm
		if doubledError > -deltaRow {
			errorTerm -= deltaRow
			startColumn += stepColumn
			appendPoint(startColumn, startRow)
		}
		if doubledError < deltaColumn {
			errorTerm += deltaColumn
			startRow += stepRow
			appendPoint(startColumn, startRow)
		}
	}
	return path
}

func riverPathSelfSeparated(path []int, width, separation int) bool {
	separation = maxInt(2, separation)
	buckets := make(map[image.Point][]int)
	seen := make(map[int]struct{}, len(path))
	for position, index := range path {
		if _, exists := seen[index]; exists {
			return false
		}
		seen[index] = struct{}{}
		column, row := index%width, index/width
		cell := image.Pt(column/separation, row/separation)
		for cellRow := cell.Y - 1; cellRow <= cell.Y+1; cellRow++ {
			for cellColumn := cell.X - 1; cellColumn <= cell.X+1; cellColumn++ {
				for _, previousPosition := range buckets[image.Pt(cellColumn, cellRow)] {
					if position-previousPosition <= separation*4 {
						continue
					}
					previous := path[previousPosition]
					if absInt(column-previous%width) < separation && absInt(row-previous/width) < separation {
						return false
					}
				}
			}
		}
		if position%maxInt(1, separation/3) == 0 {
			buckets[cell] = append(buckets[cell], position)
		}
	}
	return true
}

func riverPathTouchesWater(flow []uint32, path []int, width, height, radius, startAllowance, endAllowance int, threshold uint32) bool {
	for position, index := range path {
		if position < startAllowance || position >= len(path)-endAllowance {
			continue
		}
		column, row := index%width, index/width
		for nearbyRow := maxInt(0, row-radius); nearbyRow <= minInt(height-1, row+radius); nearbyRow++ {
			for nearbyColumn := maxInt(0, column-radius); nearbyColumn <= minInt(width-1, column+radius); nearbyColumn++ {
				if flow[tileIndex(nearbyColumn, nearbyRow, width)] >= threshold {
					return true
				}
			}
		}
	}
	return false
}
