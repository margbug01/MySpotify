// Command icon draws the app icons: the layers of AppIcon.icon, the Icon
// Composer document that macOS 26 and later render in Liquid Glass, and
// resources/icon.png, the same icon as macOS draws it, on the grid of older
// macOS icons, for older macOS and Windows; and from AppIcon-dev.icon, cream
// where the app's is red, resources/icon-dev.png, the icon of mygo dev
// builds.
//
// Run it from the repository root on a Mac with Xcode 26 or later:
//
//	go run ./icon
//
// The icon.json of each document, its background, glass and dark-mode
// colors, is edited by hand or in Icon Composer.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const ictool = "/Applications/Xcode.app/Contents/Applications/Icon Composer.app/Contents/Executables/ictool"

// icons are the documents drawn, with the PNG rendered from each and the
// colors of its tonearm: the gradient and the cap on its pivot.
var icons = []struct {
	document, output   string
	top, bottom, pivot string
}{
	// Red, with a white tonearm.
	{"icon/AppIcon.icon", "resources/icon.png", "#FFFFFF", "#E6E3E5", "#D9D5D8"},
	// Cream, with a red one to match the label.
	{"icon/AppIcon-dev.icon", "resources/icon-dev.png", "#FF5B5B", "#E3122C", "#FFE1DE"},
}

// A record on Icon Composer's 1024-point full-bleed canvas, seen close up:
// larger than the canvas, it runs off the left and bottom edges. And the
// tonearm over it, from its pivot to the headshell resting on the grooves.
var (
	rec   = struct{ cx, cy, r float64 }{330, 712, 560}
	pivot = image.Pt(820, 214)
	head  = image.Pt(585, 520)
)

func main() {
	for _, ic := range icons {
		layers := map[string]string{
			"arm.svg":    armSVG(ic.top, ic.bottom, ic.pivot),
			"record.svg": recordSVG(),
			"label.svg":  labelSVG(),
		}
		for name, s := range layers {
			if err := os.WriteFile(filepath.Join(ic.document, "Assets", name), []byte(s), 0o644); err != nil {
				log.Fatal(err)
			}
		}
		if err := render(ic.document, ic.output); err != nil {
			log.Fatal(err)
		}
	}
}

func svg(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024">` + "\n" + body + "\n</svg>\n"
}

func n(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

func armSVG(top, bottom, cap string) string {
	p := pivot
	length := math.Hypot(float64(head.X-p.X), float64(head.Y-p.Y))
	angle := math.Atan2(float64(head.Y-p.Y), float64(head.X-p.X)) * 180 / math.Pi
	end := float64(p.X) + length - 30
	return svg(fmt.Sprintf(`<defs>
<linearGradient id="m" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="%[11]s"/><stop offset="1" stop-color="%[12]s"/></linearGradient>
</defs>
<g transform="rotate(%[3]s %[1]d %[2]d)">
<rect x="%[4]d" y="%[5]d" width="64" height="52" rx="15" fill="url(#m)"/>
<rect x="%[1]d" y="%[6]d" width="%[7]s" height="34" rx="17" fill="url(#m)"/>
<rect x="%[8]s" y="%[9]d" width="92" height="56" rx="14" fill="url(#m)" transform="rotate(28 %[10]s %[2]d)"/>
</g>
<circle cx="%[1]d" cy="%[2]d" r="72" fill="url(#m)"/>
<circle cx="%[1]d" cy="%[2]d" r="27" fill="%[13]s"/>`,
		p.X, p.Y, n(angle), p.X-104, p.Y-26, p.Y-17, n(length-30), n(end-22), p.Y-28, n(end), top, bottom, cap))
}

func recordSVG() string {
	var b strings.Builder
	fmt.Fprintf(&b, `<defs>
<radialGradient id="d" cx="%[1]s" cy="%[2]s" r="%[3]s" gradientUnits="userSpaceOnUse">
<stop offset="0.35" stop-color="#2A2A30"/><stop offset="0.95" stop-color="#17171B"/><stop offset="1" stop-color="#0E0E11"/></radialGradient>
</defs>
<circle cx="%[1]s" cy="%[2]s" r="%[3]s" fill="url(#d)"/>
<circle cx="%[1]s" cy="%[2]s" r="%[4]s" fill="none" stroke="#FFFFFF" stroke-opacity="0.14" stroke-width="3"/>
`, n(rec.cx), n(rec.cy), n(rec.r), n(rec.r-8))
	// Grooves, unevenly bright like the bands of tracks.
	for i, r := 0, rec.r*0.42; r < rec.r-12; i, r = i+1, r+7 {
		fmt.Fprintf(&b, `<circle cx="%s" cy="%s" r="%s" fill="none" stroke="#FFFFFF" stroke-opacity="%s" stroke-width="1.4"/>`+"\n",
			n(rec.cx), n(rec.cy), n(r), n([]float64{0.05, 0.08, 0.04, 0.07}[i%4]))
	}
	b.WriteString(sheen(-62, 54, 0.2, rec.r*0.39, rec.r-10))
	b.WriteString(sheen(118, 54, 0.14, rec.r*0.39, rec.r-10))
	return svg(strings.TrimSuffix(b.String(), "\n"))
}

// sheen is a soft band of reflected light across the grooves, centered on
// the angle mid, in degrees: slices whose opacity falls off smoothly from
// the band's middle, as SVG has no conic gradient.
func sheen(mid, width, peak, r0, r1 float64) string {
	const slices = 48
	pt := func(a, r float64) string {
		a *= math.Pi / 180
		return n(rec.cx+math.Cos(a)*r) + " " + n(rec.cy+math.Sin(a)*r)
	}
	var b strings.Builder
	for i := range slices {
		a0 := mid - width/2 + width*float64(i)/slices
		a1 := a0 + width/slices + 0.15 // overlap, so no seams show
		t := (float64(i)+0.5)/slices*2 - 1
		o := math.Round(peak*math.Exp(-t*t*3.2)*1000) / 1000
		fmt.Fprintf(&b, `<path d="M%sL%sA%s %s 0 0 1 %sL%sA%s %s 0 0 0 %sZ" fill="#FFFFFF" fill-opacity="%s"/>`+"\n",
			pt(a0, r0), pt(a0, r1), n(r1), n(r1), pt(a1, r1), pt(a1, r0), n(r0), n(r0), pt(a0, r0), n(o))
	}
	return b.String()
}

func labelSVG() string {
	return svg(fmt.Sprintf(`<defs>
<linearGradient id="l" x1="0" y1="0" x2="0" y2="1">
<stop offset="0" stop-color="#FF5B5B"/><stop offset="1" stop-color="#E3122C"/></linearGradient>
</defs>
<circle cx="%[1]s" cy="%[2]s" r="%[3]s" fill="url(#l)"/>
<circle cx="%[1]s" cy="%[2]s" r="%[4]s" fill="none" stroke="#FFFFFF" stroke-opacity="0.28" stroke-width="3"/>
<circle cx="%[1]s" cy="%[2]s" r="%[5]s" fill="#FFF6F2"/>`, n(rec.cx), n(rec.cy), n(rec.r*0.358), n(rec.r*0.247), n(rec.r*0.052)))
}

// render has ictool draw document as macOS does, and lays it on the
// 1024-pixel grid of macOS icons at output: an 824-pixel body, centered,
// over a soft shadow.
func render(document, output string) error {
	const canvas, body = 1024, 824
	dir, err := os.MkdirTemp("", "icon")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	drawn := filepath.Join(dir, "icon.png")
	size := fmt.Sprint(body)
	if out, err := exec.Command(ictool, document, "--export-image", "--output-file", drawn,
		"--platform", "macOS", "--rendition", "Default", "--width", size, "--height", size, "--scale", "1").CombinedOutput(); err != nil {
		return fmt.Errorf("ictool (it comes with Xcode 26 or later): %v\n%s", err, out)
	}
	// ictool draws in Display P3, and the PNG this writes carries no
	// profile, which readers take as sRGB: convert, or the reds go dull.
	if out, err := exec.Command("sips", "--matchTo", "/System/Library/ColorSync/Profiles/sRGB Profile.icc", drawn).CombinedOutput(); err != nil {
		return fmt.Errorf("sips: %v\n%s", err, out)
	}
	f, err := os.Open(drawn)
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := png.Decode(f)
	if err != nil {
		return err
	}

	at := image.Pt((canvas-body)/2, (canvas-body)/2)
	dst := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	shadow := image.NewAlpha(dst.Bounds())
	draw.Draw(shadow, src.Bounds().Add(at).Add(image.Pt(0, 12)), src, src.Bounds().Min, draw.Src)
	blur(shadow, 10)
	draw.DrawMask(dst, dst.Bounds(), image.NewUniform(color.NRGBA{A: 72}), image.Point{}, shadow, image.Point{}, draw.Over)
	draw.Draw(dst, src.Bounds().Add(at), src, src.Bounds().Min, draw.Over)

	out, err := os.Create(output)
	if err != nil {
		return err
	}
	if err := png.Encode(out, dst); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// blur approximates a Gaussian blur of the given radius with three box
// blurs in each direction.
func blur(m *image.Alpha, radius int) {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	buf := make([]uint8, len(m.Pix))
	pass := func(src, dst []uint8, count, length, stride, step int) {
		for line := range count {
			base := line * stride
			sum := 0
			for i := -radius; i <= radius; i++ {
				sum += int(src[base+min(max(i, 0), length-1)*step])
			}
			for i := range length {
				dst[base+i*step] = uint8(sum / (2*radius + 1))
				sum += int(src[base+min(i+radius+1, length-1)*step]) - int(src[base+max(i-radius, 0)*step])
			}
		}
	}
	for range 3 {
		pass(m.Pix, buf, h, w, m.Stride, 1)
		pass(buf, m.Pix, w, h, 1, m.Stride)
	}
}
