package main

import (
	"image"
	"math"
	"math/rand"
	"sort"
)

type blobPoint struct{ X, Y float64 }

func (p blobPoint) add(q blobPoint) blobPoint      { return blobPoint{p.X + q.X, p.Y + q.Y} }
func (p blobPoint) sub(q blobPoint) blobPoint      { return blobPoint{p.X - q.X, p.Y - q.Y} }
func (p blobPoint) scale(factor float64) blobPoint { return blobPoint{p.X * factor, p.Y * factor} }
func (p blobPoint) length() float64                { return math.Hypot(p.X, p.Y) }
func (p blobPoint) unit() blobPoint {
	if length := p.length(); length > 0 {
		return p.scale(1 / length)
	}
	return blobPoint{}
}
func (p blobPoint) tile() image.Point { return image.Pt(int(math.Floor(p.X)), int(math.Floor(p.Y))) }

type blobNode struct {
	point    blobPoint
	angle    float64
	depth    int
	adjacent [3]int
	degree   int
}

func genBlobTree(origin blobPoint, size float64, maxNodes, maxDepth int, rng *rand.Rand) []blobNode {
	nodes := make([]blobNode, 1, maxNodes)
	nodes[0] = blobNode{point: origin, angle: rng.Float64() * 360}
	for head := 0; head < len(nodes); head++ {
		parent := nodes[head]
		if parent.depth >= maxDepth || len(nodes) >= maxNodes {
			continue
		}
		if len(nodes) >= 4 && rng.Float64() >= 0.5 {
			continue
		}
		children := 1
		if rng.Float64() >= 0.8 {
			children = 2
		}
		sector := 360 / float64(children+1)
		start := parent.angle + float64(children)*180/float64(children+1)
		for child := 0; child < children && len(nodes) < maxNodes; child++ {
			angle := start - rng.Float64()*sector
			start -= sector
			length := size * (0.9 + 0.2*rng.Float64())
			radians := angle * math.Pi / 180
			point := parent.point.add(blobPoint{math.Cos(radians) * length, -math.Sin(radians) * length})
			nodes[head].adjacent[nodes[head].degree] = len(nodes)
			nodes[head].degree++
			nodes = append(nodes, blobNode{point: point, angle: angle, depth: parent.depth + 1, adjacent: [3]int{head}, degree: 1})
		}
	}
	return nodes
}

func genBlobOutline(nodes []blobNode, width float64, rng *rand.Rand) []blobPoint {
	outline := make([]blobPoint, 0, 3*len(nodes))
	var walk func(int, bool)
	walk = func(index int, first bool) {
		node := nodes[index]
		randomWidth := func() float64 { return width * (0.8 + 0.4*rng.Float64()) }
		switch node.degree {
		case 0:
			outline = append(outline, node.point)
		case 1:
			direction := nodes[node.adjacent[0]].point.sub(node.point).unit().scale(randomWidth())
			normal := blobPoint{-direction.Y, direction.X}
			outline = append(outline, node.point.add(normal), node.point.sub(direction), node.point.sub(normal))
			if first {
				walk(node.adjacent[0], false)
			}
		case 2:
			direction := nodes[node.adjacent[0]].point.sub(nodes[node.adjacent[1]].point).unit().scale(randomWidth())
			normal := blobPoint{-direction.Y, direction.X}
			outline = append(outline, node.point.add(normal))
			walk(node.adjacent[1], false)
			outline = append(outline, node.point.sub(normal))
			if first {
				walk(node.adjacent[0], false)
			}
		default:
			for i := 0; i < node.degree; i++ {
				next := node.adjacent[(i+1)%node.degree]
				direction := nodes[node.adjacent[i]].point.sub(nodes[next].point).unit().scale(randomWidth())
				outline = append(outline, node.point.add(blobPoint{-direction.Y, direction.X}))
				if first || i != node.degree-1 {
					walk(next, false)
				}
			}
		}
	}
	walk(0, true)
	return outline
}

func blobSpline(outline []blobPoint, width, raggedness float64, rng *rand.Rand) []blobPoint {
	points := make([]blobPoint, 0, len(outline))
	for _, point := range outline {
		if len(points) == 0 || point != points[len(points)-1] {
			points = append(points, point)
		}
	}
	if len(points) > 1 && points[0] == points[len(points)-1] {
		points = points[:len(points)-1]
	}
	if len(points) == 0 {
		return nil
	}
	tangent := func(i int) blobPoint {
		return points[(i+1)%len(points)].sub(points[(i+len(points)-1)%len(points)]).unit().scale(width)
	}
	subdivisions := func(i int) int {
		return max(1, int(math.Ceil((points[(i+1)%len(points)].sub(points[i]).length()+2*width)/3)))
	}
	count := 1
	for i := range points {
		count += subdivisions(i)
	}
	contour := make([]blobPoint, 0, count)
	for i, start := range points {
		end := points[(i+1)%len(points)]
		startTangent, endTangent := tangent(i), tangent((i+1)%len(points))
		steps := subdivisions(i)
		for step := 0; step < steps; step++ {
			fraction := float64(step) / float64(steps)
			square := fraction * fraction
			cube := square * fraction
			point := start.scale(2*cube - 3*square + 1).add(startTangent.scale(cube - 2*square + fraction)).add(end.scale(-2*cube + 3*square)).add(endTangent.scale(cube - square))
			if raggedness > 0 {
				point = point.add(blobPoint{(2*rng.Float64() - 1) * raggedness, (2*rng.Float64() - 1) * raggedness})
			}
			contour = append(contour, point)
		}
	}
	return append(contour, contour[0])
}

type blobMask struct {
	bounds image.Rectangle
	cells  []byte
}

func (m blobMask) contains(point image.Point) bool {
	return point.In(m.bounds) && m.cells[(point.Y-m.bounds.Min.Y)*m.bounds.Dx()+point.X-m.bounds.Min.X] != 0
}
func (m blobMask) set(point image.Point) {
	if point.In(m.bounds) {
		m.cells[(point.Y-m.bounds.Min.Y)*m.bounds.Dx()+point.X-m.bounds.Min.X] = 1
	}
}
func (m blobMask) each(visit func(image.Point)) {
	for i, cell := range m.cells {
		if cell != 0 {
			visit(image.Pt(m.bounds.Min.X+i%m.bounds.Dx(), m.bounds.Min.Y+i/m.bounds.Dx()))
		}
	}
}

type blobScratch struct {
	cells         []byte
	queue         []int
	intersections []float64
}

func (s *blobScratch) rasterize(nodes []blobNode, contour []blobPoint, width float64, world image.Rectangle) blobMask {
	radius := 0.8 * width
	low, high := nodes[0].point, nodes[0].point
	include := func(p blobPoint) {
		low.X = math.Min(low.X, p.X)
		low.Y = math.Min(low.Y, p.Y)
		high.X = math.Max(high.X, p.X)
		high.Y = math.Max(high.Y, p.Y)
	}
	for _, p := range contour {
		include(p)
	}
	for _, node := range nodes {
		include(node.point.sub(blobPoint{radius, radius}))
		include(node.point.add(blobPoint{radius, radius}))
	}
	bounds := image.Rect(int(math.Floor(low.X)), int(math.Floor(low.Y)), int(math.Floor(high.X))+1, int(math.Floor(high.Y))+1).Intersect(world)
	count := bounds.Dx() * bounds.Dy()
	if cap(s.cells) < count {
		s.cells = make([]byte, count)
	} else {
		s.cells = s.cells[:count]
		clear(s.cells)
	}
	if cap(s.queue) < count {
		s.queue = make([]int, 0, count)
	} else {
		s.queue = s.queue[:0]
	}
	mask := blobMask{bounds, s.cells}
	for row := bounds.Min.Y; row < bounds.Max.Y; row++ {
		scanY := float64(row) + 0.5
		s.intersections = s.intersections[:0]
		for i := 1; i < len(contour); i++ {
			a, b := contour[i-1], contour[i]
			if (a.Y <= scanY && b.Y > scanY) || (b.Y <= scanY && a.Y > scanY) {
				s.intersections = append(s.intersections, a.X+(scanY-a.Y)*(b.X-a.X)/(b.Y-a.Y))
			}
		}
		sort.Float64s(s.intersections)
		for i := 1; i < len(s.intersections); i += 2 {
			start := max(bounds.Min.X, int(math.Ceil(s.intersections[i-1]-0.5)))
			end := min(bounds.Max.X, int(math.Ceil(s.intersections[i]-0.5)))
			for column := start; column < end; column++ {
				mask.set(image.Pt(column, row))
			}
		}
	}
	for i := 1; i < len(contour); i++ {
		blobSupercover(contour[i-1], contour[i], mask.set)
	}
	for i, node := range nodes {
		mask.set(node.point.tile())
		if i == 0 {
			continue
		}
		parent := nodes[node.adjacent[0]].point
		rasterBlobCapsule(mask, parent, node.point, radius)
		blobSupercover(parent, node.point, mask.set)
	}
	seed := nodes[0].point.tile()
	mask.set(seed)
	if !seed.In(bounds) {
		return mask
	}
	seedIndex := (seed.Y-bounds.Min.Y)*bounds.Dx() + seed.X - bounds.Min.X
	mask.cells[seedIndex] = 2
	s.queue = append(s.queue, seedIndex)
	for head := 0; head < len(s.queue); head++ {
		index := s.queue[head]
		column, row := index%bounds.Dx(), index/bounds.Dx()
		for _, delta := range blobDirections {
			nextColumn, nextRow := column+delta.X, row+delta.Y
			if nextColumn < 0 || nextRow < 0 || nextColumn >= bounds.Dx() || nextRow >= bounds.Dy() {
				continue
			}
			next := nextRow*bounds.Dx() + nextColumn
			if mask.cells[next] == 1 {
				mask.cells[next] = 2
				s.queue = append(s.queue, next)
			}
		}
	}
	for i, value := range mask.cells {
		if value == 2 {
			mask.cells[i] = 1
		} else {
			mask.cells[i] = 0
		}
	}
	return mask
}

var blobDirections = [4]image.Point{{X: -1}, {X: 1}, {Y: -1}, {Y: 1}}

// A grid traversal includes both side cells at a corner, keeping diagonal edges 4-connected.
func blobSupercover(start, end blobPoint, visit func(image.Point)) {
	current, target := start.tile(), end.tile()
	visit(current)
	delta := end.sub(start)
	stepX, stepY := 1, 1
	if delta.X < 0 {
		stepX = -1
	}
	if delta.Y < 0 {
		stepY = -1
	}
	crossing := func(coordinate, delta float64) (float64, float64) {
		if delta == 0 {
			return math.Inf(1), math.Inf(1)
		}
		boundary := math.Floor(coordinate) + 1
		if delta < 0 {
			boundary = math.Floor(coordinate)
		}
		return (boundary - coordinate) / delta, math.Abs(1 / delta)
	}
	nextX, strideX := crossing(start.X, delta.X)
	nextY, strideY := crossing(start.Y, delta.Y)
	for current != target {
		if current.X == target.X {
			current.Y += stepY
			nextY += strideY
		} else if current.Y == target.Y {
			current.X += stepX
			nextX += strideX
		} else if nextX < nextY {
			current.X += stepX
			nextX += strideX
		} else if nextY < nextX {
			current.Y += stepY
			nextY += strideY
		} else {
			visit(image.Pt(current.X+stepX, current.Y))
			visit(image.Pt(current.X, current.Y+stepY))
			current.X += stepX
			current.Y += stepY
			nextX += strideX
			nextY += strideY
		}
		visit(current)
	}
}

func rasterBlobCapsule(mask blobMask, start, end blobPoint, radius float64) {
	bounds := image.Rect(int(math.Floor(math.Min(start.X, end.X)-radius)), int(math.Floor(math.Min(start.Y, end.Y)-radius)), int(math.Floor(math.Max(start.X, end.X)+radius))+1, int(math.Floor(math.Max(start.Y, end.Y)+radius))+1).Intersect(mask.bounds)
	delta := end.sub(start)
	lengthSquared := delta.X*delta.X + delta.Y*delta.Y
	for row := bounds.Min.Y; row < bounds.Max.Y; row++ {
		for column := bounds.Min.X; column < bounds.Max.X; column++ {
			point := blobPoint{float64(column) + 0.5, float64(row) + 0.5}
			relative := point.sub(start)
			fraction := 0.0
			if lengthSquared > 0 {
				fraction = clampFloat((relative.X*delta.X+relative.Y*delta.Y)/lengthSquared, 0, 1)
			}
			distance := point.sub(start.add(delta.scale(fraction)))
			if distance.X*distance.X+distance.Y*distance.Y <= radius*radius {
				mask.set(image.Pt(column, row))
			}
		}
	}
}

const (
	blobMainPass         uint64 = 0xb406a72f4139d085
	blobSecondaryPass    uint64 = 0x83c01d29ea749f65
	blobPositionPurpose  uint64 = 0x7158264d
	blobSelectionPurpose uint64 = 0x314859ab
	blobSizePurpose      uint64 = 0x8657412c
	blobGeometryPurpose  uint64 = 0x1f0398ad
	blobIsletPurpose     uint64 = 0xe9a2b547
)

func blobRandom(seed int64, pass uint64, column, row int, purpose uint64) *rand.Rand {
	mix := func(value uint64) uint64 {
		value ^= value >> 30
		value *= 0xbf58476d1ce4e5b9
		value ^= value >> 27
		value *= 0x94d049bb133111eb
		return value ^ (value >> 31)
	}
	mixed := mix(uint64(seed) ^ pass)
	mixed = mix(mixed ^ uint64(column))
	mixed = mix(mixed ^ uint64(row))
	mixed = mix(mixed ^ purpose)
	return rand.New(rand.NewSource(int64(mixed)))
}

func blobWidth(size, configured float64) float64 {
	if configured > 0 {
		return configured
	}
	return size / 2
}
