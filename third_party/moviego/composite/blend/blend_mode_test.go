package blend

import "testing"

// Test Mix1 formulas per standard definitions (values hand-computed).
func TestMix1Formulas(t *testing.T) {
        cases := []struct {
                fg, bg int
                mode   Mode
                want   int
        }{
                // screen: 255 - (255-fg)(255-bg)/255
                {200, 100, ModeScreen, 255 - (55*155+127)/255},  // ≈222
                {0, 0, ModeScreen, 255 - (255*255+127)/255},     // 0
                {255, 255, ModeScreen, 255},                     // trắng giữ trắng
                // multiply: fg*bg/255
                {200, 100, ModeMultiply, (200*100 + 127) / 255}, // ≈78
                {255, 100, ModeMultiply, 100},                   // trắng = đồng nhất
                {0, 100, ModeMultiply, 0},                       // đen = đen
                // overlay phụ thuộc bg < 128
                {200, 100, ModeOverlay, (2*200*100 + 127) / 255},             // bg tối → 157
                {200, 200, ModeOverlay, 255 - (2*55*55+127)/255},             // bg sáng
                // darken / lighten
                {200, 100, ModeDarken, 100},
                {50, 100, ModeDarken, 50},
                {200, 100, ModeLighten, 200},
                {250, 100, ModeLighten, 250},
                // add bão hòa ở 255
                {200, 100, ModeAdd, 255},
                {100, 100, ModeAdd, 200},
        }
        for _, c := range cases {
                got := int(mix1(c.fg, c.bg, c.mode))
                if got != c.want {
                        t.Errorf("mix1(%d,%d,mode %d) = %d, muốn %d", c.fg, c.bg, c.mode, got, c.want)
                }
        }
}

// Test BlendRow preserves destination outside the source and changes it inside.
func TestBlendRowScreen(t *testing.T) {
        dst := []byte{100, 100, 100, 100, 100, 100} // 2 px
        fg := []byte{255, 255, 255, 0, 0, 0}
        BlendRow(dst, fg, 2, ModeScreen)
        // screen(100,255)=255 ; screen(100,0)=100
        if dst[0] != 255 || dst[1] != 255 || dst[2] != 255 {
                t.Fatalf("screen với fg trắng phải ra trắng, nhận %v", dst[:3])
        }
        if dst[3] != 100 || dst[4] != 100 || dst[5] != 100 {
                t.Fatalf("screen với fg đen phải giữ nền, nhận %v", dst[3:6])
        }
}

// Test BlendRowMasked: alpha 0 → nền không đổi; alpha 255 → công thức đầy đủ.
func TestBlendRowMaskedAlpha(t *testing.T) {
        dst := []byte{100, 100, 100}
        fg := []byte{255, 255, 255}
        a := []byte{0}
        BlendRowMasked(dst, fg, a, 1, ModeScreen)
        if dst[0] != 100 {
                t.Fatalf("alpha 0 phải giữ nền, nhận %d", dst[0])
        }
        a[0] = 255
        BlendRowMasked(dst, fg, a, 1, ModeScreen)
        if dst[0] != 255 {
                t.Fatalf("alpha 255 + screen trắng phải ra 255, nhận %d", dst[0])
        }
}
