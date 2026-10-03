package blend

// Blend1 blends one channel: fg*a + bg*(255-a), divided by 255 with rounding.
// a is the source opacity in 0..255. The (t + (t>>8))>>8 trick rounds the
// divide-by-255 without a real division.
func Blend1(fg, bg, a byte) byte {
        t := int(fg)*int(a) + int(bg)*(255-int(a)) + 128
        return byte((t + (t >> 8)) >> 8)
}

// CopyRGBRow copies n packed RGB24 pixels from src into dst (path a, and the
// pixel half of path b).
func CopyRGBRow(dst, src []byte, n int) {
        copy(dst[:n*3], src[:n*3])
}

// LerpRow cross-fades two packed rows into dst: dst = b*t + a*(255-t) per byte,
// with t the 0..255 weight toward b. It is the time-domain analogue of the
// spatial "over" kernels — the per-row blend a crossfade transition is built on
// — and works for both RGB24 and Gray8 rows since it operates per byte. n is the
// byte count (pixels*bpp).
func LerpRow(dst, a, b []byte, n int, t byte) {
        switch t {
        case 0:
                copy(dst[:n], a[:n])
        case 255:
                copy(dst[:n], b[:n])
        default:
                for i := 0; i < n; i++ {
                        dst[i] = Blend1(b[i], a[i], t)
                }
        }
}

// FillAlphaRow sets n Gray8 alpha bytes to 255, marking the region the source
// covered as fully opaque (path b).
func FillAlphaRow(dst []byte, n int) {
        for i := 0; i < n; i++ {
                dst[i] = 255
        }
}

// OverOpaqueRow composites a masked source over an opaque destination (path c):
// dst = fg*a + bg*(255-a), per channel. dst and fg are RGB24 rows of n pixels;
// a is the source's Gray8 alpha row. The destination stays opaque (no alpha).
func OverOpaqueRow(dst, fg, a []byte, n int) {
        for x := 0; x < n; x++ {
                o := x * 3
                av := a[x]
                dst[o] = Blend1(fg[o], dst[o], av)
                dst[o+1] = Blend1(fg[o+1], dst[o+1], av)
                dst[o+2] = Blend1(fg[o+2], dst[o+2], av)
        }
}

// OverRow composites a masked source over a masked destination (path d), the
// "over" operator with premultiplied recovery:
//
//      final_a = a + a_bg*(1-a)
//      out     = (fg*a + bg*a_bg*(1-a)) / final_a      (final_a==0 -> transparent)
//
// All alpha is integer 0..255. dstRGB/fgRGB are RGB24 rows; aDst is the
// destination Gray8 alpha (updated in place to final_a); aSrc is the source
// Gray8 alpha. n is the pixel count. The division is rounded so the result
// stays within 1 of the float64 reference.
func OverRow(dstRGB, fgRGB, aSrc, aDst []byte, n int) {
        for x := 0; x < n; x++ {
                a := int(aSrc[x])
                abg := int(aDst[x])
                // a_bg*(1-a) scaled to 0..255, rounded.
                inner := (abg*(255-a) + 127) / 255
                finalA := a + inner
                if finalA <= 0 {
                        // Fully transparent pixel: leave RGB as-is, clear alpha.
                        aDst[x] = 0
                        continue
                }
                o := x * 3
                half := finalA / 2
                for c := 0; c < 3; c++ {
                        num := int(fgRGB[o+c])*a + int(dstRGB[o+c])*inner
                        dstRGB[o+c] = byte((num + half) / finalA)
                }
                if finalA > 255 {
                        finalA = 255
                }
                aDst[x] = byte(finalA)
        }
}

// Mode identifies a creative blend mode: how a foreground child mixes with the
// layers below it inside a composite. ModeNormal is the classic alpha-over
// operator; the other modes are the standard separable formulas used by image
// editors (screen, multiply, overlay, darken, lighten, add).
type Mode int

const (
        ModeNormal Mode = iota
        ModeScreen
        ModeMultiply
        ModeOverlay
        ModeDarken
        ModeLighten
        ModeAdd
)

// Valid reports whether m is a known non-normal blend mode.
func (m Mode) Valid() bool { return m > ModeNormal && m <= ModeAdd }

// mix1 returns the blend of one channel pair: the mode formula. The result is
// later weighted by the foreground alpha by the caller.
func mix1(fg, bg int, m Mode) int {
        switch m {
        case ModeScreen:
                return 255 - ((255-fg)*(255-bg)+127)/255
        case ModeMultiply:
                return (fg*bg + 127) / 255
        case ModeOverlay:
                if bg < 128 {
                        return (2 * fg * bg + 127) / 255
                }
                return 255 - ((2*(255-fg)*(255-bg))+127)/255
        case ModeDarken:
                if fg < bg {
                        return fg
                }
                return bg
        case ModeLighten:
                if fg > bg {
                        return fg
                }
                return bg
        case ModeAdd:
                v := fg + bg
                if v > 255 {
                        return 255
                }
                return v
        default:
                return fg
        }
}

// BlendRow blends an opaque source row into the destination (the composite
// path-a analogue for creative modes): dst = blend(fg over dst) per pixel.
// dst and fg are RGB24 rows of n pixels. The destination stays opaque.
func BlendRow(dst, fg []byte, n int, m Mode) {
        if !m.Valid() {
                CopyRGBRow(dst, fg, n)
                return
        }
        for x := 0; x < n; x++ {
                o := x * 3
                dst[o] = byte(mix1(int(fg[o]), int(dst[o]), m))
                dst[o+1] = byte(mix1(int(fg[o+1]), int(dst[o+1]), m))
                dst[o+2] = byte(mix1(int(fg[o+2]), int(dst[o+2]), m))
        }
}

// BlendRowMasked blends a masked source row into an opaque destination (the
// path-c analogue for creative modes): where the source alpha is a, the
// destination becomes a mix between the untouched background and the mode
// formula result, so alpha still controls how strongly the layer blends.
func BlendRowMasked(dst, fg, a []byte, n int, m Mode) {
        if !m.Valid() {
                OverOpaqueRow(dst, fg, a, n)
                return
        }
        for x := 0; x < n; x++ {
                av := int(a[x])
                o := x * 3
                for c := 0; c < 3; c++ {
                        blended := mix1(int(fg[o+c]), int(dst[o+c]), m)
                        // Lerp background toward the blended result by av/255.
                        t := (av*blended + (255-av)*int(dst[o+c]) + 127) / 255
                        dst[o+c] = byte(t)
                }
        }
}
