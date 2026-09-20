package render

import (
	"bytes"
	"fmt"
	"image/color"
	"testing"
	"time"
)

// A path-painting operator ENDS the path object: once the path is painted it
// "shall be reset" (PDF 32000-1:2008, 8.5.3.1). Only "n" did that here, so each
// painting operator re-painted everything drawn before it.
//
// That is visible in one page with two strokes: the second S re-strokes the first
// segment in the SECOND colour, so a red line comes out blue. It is also quadratic:
// a gnuplot matrix plot with 49463 "m l S" triples painted 1.2 billion segments and
// did not finish in ten minutes (0.1s with this fix).
func twoStrokePDF() []byte {
	content := []byte("2 w\n1 0 0 RG\n20 20 m 20 80 l S\n0 0 1 RG\n60 20 m 60 80 l S\n")
	objs := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Contents 4 0 R /Resources << >> >>"),
		append([]byte(fmt.Sprintf("<< /Length %d >>\nstream\n", len(content))),
			append(content, []byte("endstream")...)...),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offs := make([]int, 0, len(objs))
	for i, o := range objs {
		offs = append(offs, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objs)+1, xref)
	return out.Bytes()
}

func TestPaintingOperatorEndsThePath(t *testing.T) {
	r, err := NewRenderer(twoStrokePDF())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	img, err := r.RenderPage(1, 72)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	// The two strokes are vertical lines at x=20 (red) and x=60 (blue).
	for _, c := range []struct {
		x    int
		want color.RGBA
		name string
	}{
		{20, color.RGBA{255, 0, 0, 255}, "first stroke stays its own colour"},
		{60, color.RGBA{0, 0, 255, 255}, "second stroke"},
	} {
		got := img.At(b.Min.X+c.x, b.Min.Y+50)
		gr, gg, gb, _ := got.RGBA()
		if uint8(gr>>8) != c.want.R || uint8(gg>>8) != c.want.G || uint8(gb>>8) != c.want.B {
			t.Errorf("%s: x=%d is rgb(%d,%d,%d), want rgb(%d,%d,%d)",
				c.name, c.x, gr>>8, gg>>8, gb>>8, c.want.R, c.want.G, c.want.B)
		}
	}
}

// The same defect measured as time rather than colour: N painting operators did
// O(N^2) work. Ten thousand strokes finish in well under a second once each S ends
// its path; before the fix this test does not finish.
func TestManyStrokesAreLinear(t *testing.T) {
	var content bytes.Buffer
	content.WriteString("0.5 w\n")
	const n = 10000
	for i := 0; i < n; i++ {
		fmt.Fprintf(&content, "%d 10 m %d 90 l S\n", i%500, i%500)
	}
	body := content.Bytes()
	objs := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 500 100] /Contents 4 0 R /Resources << >> >>"),
		append([]byte(fmt.Sprintf("<< /Length %d >>\nstream\n", len(body))),
			append(body, []byte("endstream")...)...),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offs := make([]int, 0, len(objs))
	for i, o := range objs {
		offs = append(offs, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offs {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objs)+1, xref)

	r, err := NewRenderer(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	start := time.Now()
	if _, err := r.RenderPage(1, 72); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Errorf("%d strokes took %s — the path is not being reset between them", n, d)
	}
}
