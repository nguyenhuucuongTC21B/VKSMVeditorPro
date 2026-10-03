package composite

import (
        "github.com/mowshon/moviego/v2/clip"
        "github.com/mowshon/moviego/v2/composite/blend"
        "github.com/mowshon/moviego/v2/video"
)

// PosKeyword is a symbolic placement that overrides the numeric X/Y of a
// Position. The unspecified axis is centered (e.g. PosLeft centers vertically),
// matching MoviePy's keyword handling.
type PosKeyword int

const (
        // PosNone uses the numeric X/Y instead of a keyword.
        PosNone PosKeyword = iota
        PosCenter
        PosLeft
        PosRight
        PosTop
        PosBottom
)

// Position places a child's top-left within a composite canvas.
//
// Keyword, when set, overrides X/Y. Otherwise X/Y are absolute pixels, unless
// the matching Rel flag is set, in which case the value is a fraction of the
// free space on that axis (canvasDim - childDim): 0 is flush against the start,
// 1 against the end, 0.5 is centered. This differs from MoviePy's
// fraction-of-canvas relative model and is recorded in docs/compatibility.md.
//
// Animated, when non-nil, supplies absolute (x, y) pixels as a function of
// composition time and overrides everything else.
type Position struct {
        X, Y     float64
        RelX     bool
        RelY     bool
        Keyword  PosKeyword
        Animated func(t clip.Time) (x, y float64)
}

// CompositeChild is one layer in a composite: the clip, its placement start on
// the composite timeline, its position, its layer index (z-order; a stable
// sort by layer means equal layers keep input order), and its creative blend
// mode (ModeNormal is the classic alpha-over; see composite/blend).
type CompositeChild struct {
        Clip  video.VideoClip
        Start clip.Time
        Pos   Position
        Layer int
        Blend blend.Mode
}

// PlacementNode wraps a VideoClip with the placement metadata a composite
// consumes, so `clip.Position(...).Layer(...)` composes through the facade
// without widening the VideoClip interface. It is a passthrough for every
// VideoClip method (rendering and timeline delegate to Inner); only Child
// carries the extra placement state.
type PlacementNode struct {
        video.VideoClip
        Child CompositeChild
}

// WithPosition returns a copy of n whose child position is pos.
func (n *PlacementNode) WithPosition(pos Position) *PlacementNode {
        c := *n
        c.Child.Pos = pos
        return &c
}

// WithLayer returns a copy of n whose child layer index is layer.
func (n *PlacementNode) WithLayer(layer int) *PlacementNode {
        c := *n
        c.Child.Layer = layer
        return &c
}

// WithBlend returns a copy of n whose child blend mode is m. The composite
// applies m when painting this child over the layers below it.
func (n *PlacementNode) WithBlend(m blend.Mode) *PlacementNode {
        c := *n
        c.Child.Blend = m
        return &c
}

// BlendMode is the creative blend mode of a child layer, an alias of
// blend.Mode re-exported for facade packages that should not import the
// low-level blend package directly.
type BlendMode = blend.Mode

// Blend modes for child layers. BlendNormal is the classic alpha-over
// operator; the rest mix the child's pixels with the layers below using the
// standard separable formulas.
const (
        BlendNormal   = blend.ModeNormal
        BlendScreen   = blend.ModeScreen
        BlendMultiply = blend.ModeMultiply
        BlendOverlay  = blend.ModeOverlay
        BlendDarken   = blend.ModeDarken
        BlendLighten  = blend.ModeLighten
        BlendAdd      = blend.ModeAdd
)

// Children exposes the wrapped clip for planner graph traversal. A placement
// over a file source is itself recognized as that source (its FileSource
// methods are promoted from the embedded clip), so this matters only for a
// placement over a composite or other non-source clip.
func (n *PlacementNode) Children() []video.VideoClip { return []video.VideoClip{n.VideoClip} }

// Place wraps a clip in a PlacementNode with default placement (origin,
// layer 0). The clip's own Start is carried into the child placement start.
func Place(c video.VideoClip) *PlacementNode {
        return &PlacementNode{
                VideoClip: c,
                Child:     CompositeChild{Clip: c, Start: c.Start()},
        }
}
