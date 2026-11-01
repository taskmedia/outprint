package images

import "github.com/taskmedia/outline-latex-pdf/internal/limits"

const (
	expansionUnvisited = iota
	expansionVisiting
	expansionDone
)

// checkExpansion counts the elements the SVG instantiates once every <use>
// is replaced by its target, and refuses a circular <use> and a count over
// the limit. A few <use> elements can name a group that names a group that
// names a group: the source stays small and the rendering does not.
func (w *svgWalker) checkExpansion() *Rejection {
	g := newExpansion(w.nodes)
	total, err := g.size(0)
	switch {
	case err != nil:
		return reject(KindSVGRejected, "the SVG has a circular <use> reference")
	case total > limits.MaxSVGExpansion:
		return reject(KindSVGRejected, "the SVG expands to more than %d elements through <use>; simplify it", limits.MaxSVGExpansion)
	}
	return nil
}

type expansion struct {
	nodes    []svgNode
	children [][]int
	byID     map[string]int
	state    []int8
	memo     []int64
}

func newExpansion(nodes []svgNode) *expansion {
	g := &expansion{
		nodes: nodes, children: make([][]int, len(nodes)), byID: map[string]int{},
		state: make([]int8, len(nodes)), memo: make([]int64, len(nodes)),
	}
	for i, n := range nodes {
		if n.parent >= 0 {
			g.children[n.parent] = append(g.children[n.parent], i)
		}
		if _, dup := g.byID[n.id]; n.id != "" && !dup {
			g.byID[n.id] = i
		}
	}
	return g
}

type circularUse struct{}

func (circularUse) Error() string { return "circular use" }

// size is the number of elements node i stands for, saturated just above the
// limit so that the arithmetic cannot overflow.
func (g *expansion) size(i int) (int64, error) {
	switch g.state[i] {
	case expansionDone:
		return g.memo[i], nil
	case expansionVisiting:
		return 0, circularUse{}
	}
	g.state[i] = expansionVisiting
	total := int64(1)
	for _, c := range g.children[i] {
		n, err := g.size(c)
		if err != nil {
			return 0, err
		}
		total = saturate(total + n)
	}
	if target, ok := g.byID[g.nodes[i].useTarget]; ok && g.nodes[i].useTarget != "" {
		n, err := g.size(target)
		if err != nil {
			return 0, err
		}
		total = saturate(total + n)
	}
	g.state[i] = expansionDone
	g.memo[i] = total
	return total, nil
}

func saturate(n int64) int64 { return min(n, limits.MaxSVGExpansion+1) }
