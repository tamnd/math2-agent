package site

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const socialWidth, socialHeight = 1200, 630

var (
	socialGold    = color.RGBA{240, 192, 91, 255}
	socialWhite   = color.RGBA{250, 249, 245, 255}
	socialMuted   = color.RGBA{187, 204, 216, 255}
	socialAccents = [...]color.RGBA{
		{69, 176, 211, 255}, {147, 120, 225, 255}, {76, 194, 155, 255}, {226, 126, 126, 255},
		{218, 168, 83, 255}, {104, 148, 233, 255}, {199, 129, 191, 255}, {98, 190, 194, 255},
	}
	socialBackgrounds                      [len(socialAccents)]*image.RGBA
	socialBackgroundOnce                   sync.Once
	socialFontOnce                         sync.Once
	socialDrawMu                           sync.Mutex
	socialBold, socialRegular, socialLabel font.Face
	socialFontErr                          error
)

func socialBackground(tag string) (*image.RGBA, color.RGBA) {
	socialBackgroundOnce.Do(func() {
		for i, accent := range socialAccents {
			bg := image.NewRGBA(image.Rect(0, 0, socialWidth, socialHeight))
			for y := 0; y < socialHeight; y++ {
				for x := 0; x < socialWidth; x++ {
					// A dark gradient, a quiet grid and concentric arcs give each
					// subject a distinct card without competing with its title.
					r := 15 + x*8/socialWidth
					g := 26 + x*10/socialWidth
					b := 43 + x*13/socialWidth
					if x > 590 {
						glow := (x - 590) * 12 / (socialWidth - 590)
						r += int(accent.R) * glow / 100
						g += int(accent.G) * glow / 100
						b += int(accent.B) * glow / 100
						if x%72 < 2 || y%72 < 2 {
							r += 4
							g += 5
							b += 6
						}
						dx, dy := x-1040, y-175
						d2 := dx*dx + dy*dy
						for _, radius := range [...]int{115, 190, 265, 340} {
							if d2 >= (radius-2)*(radius-2) && d2 <= (radius+2)*(radius+2) {
								r += int(accent.R) / 8
								g += int(accent.G) / 8
								b += int(accent.B) / 8
								break
							}
						}
					}
					bg.SetRGBA(x, y, color.RGBA{uint8(min(r, 255)), uint8(min(g, 255)), uint8(min(b, 255)), 255})
				}
			}
			draw.Draw(bg, image.Rect(0, 0, 18, socialHeight), &image.Uniform{C: accent}, image.Point{}, draw.Src)
			draw.Draw(bg, image.Rect(70, 98, 250, 104), &image.Uniform{C: socialGold}, image.Point{}, draw.Src)
			draw.Draw(bg, image.Rect(70, 526, 1130, 528), &image.Uniform{C: accent}, image.Point{}, draw.Src)
			socialBackgrounds[i] = bg
		}
	})
	i := shardOf(tag, len(socialAccents))
	return socialBackgrounds[i], socialAccents[i]
}

// socialPath names a card while the build assembles the content-addressed
// packs. A GitHub Pages build also serves this file directly.
func socialPath(tag, title, subtitle string) string {
	h := sha256.Sum256([]byte(title + "\x00" + subtitle))
	return "social/" + tag + "-" + hex.EncodeToString(h[:6]) + ".jpg"
}

func socialFace(data []byte, size float64) (font.Face, error) {
	f, err := opentype.Parse(data)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

func socialFonts() (font.Face, font.Face, font.Face, error) {
	socialFontOnce.Do(func() {
		socialBold, socialFontErr = socialFace(gobold.TTF, 66)
		if socialFontErr != nil {
			return
		}
		socialRegular, socialFontErr = socialFace(goregular.TTF, 27)
		if socialFontErr != nil {
			return
		}
		socialLabel, socialFontErr = socialFace(gobold.TTF, 26)
	})
	return socialBold, socialRegular, socialLabel, socialFontErr
}

func socialLines(face font.Face, title string, width, maxLines int) []string {
	measure := func(s string) int { return font.MeasureString(face, s).Ceil() }
	var lines []string
	line := ""
	for _, word := range strings.Fields(title) {
		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if measure(candidate) <= width {
			line = candidate
			continue
		}
		if line != "" {
			lines = append(lines, line)
			line = word
		} else {
			// Long unbroken expressions are common in mathematical titles.
			line = ""
			for _, r := range word {
				if measure(line+string(r)) > width && line != "" {
					lines = append(lines, line)
					line = ""
				}
				line += string(r)
			}
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	if len(lines) <= maxLines {
		return lines
	}
	lines = lines[:maxLines]
	last := lines[maxLines-1]
	for last != "" && measure(last+"…") > width {
		r := []rune(last)
		last = string(r[:len(r)-1])
	}
	lines[maxLines-1] = strings.TrimSpace(last) + "…"
	return lines
}

// socialJPEG renders a readable 1200×630 image for link previews. It uses
// bundled Go fonts and no system programs, so CI and the deploy server agree.
func socialJPEG(title, subtitle, tag string) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, socialWidth, socialHeight))
	background, accent := socialBackground(tag)
	draw.Draw(img, img.Bounds(), background, image.Point{}, draw.Src)
	bold, regular, label, err := socialFonts()
	if err != nil {
		return nil, err
	}
	put := func(face font.Face, ink color.Color, x, y int, s string) {
		d := font.Drawer{Dst: img, Src: &image.Uniform{C: ink}, Face: face, Dot: fixed.P(x, y)}
		d.DrawString(s)
	}
	// opentype.Face keeps a glyph cache; protect it while concurrent cards draw.
	socialDrawMu.Lock()
	put(label, socialGold, 70, 76, "MATH 2.0")
	lines := socialLines(bold, title, 1050, 3)
	for i, line := range lines {
		put(bold, socialWhite, 70, 206+i*83, line)
	}
	put(regular, accent, 70, 490, subtitle)
	put(regular, socialMuted, 70, 578, "math2.tamnd.com")
	if tag != "" {
		put(regular, socialMuted, 930, 578, tag)
	}
	socialDrawMu.Unlock()
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 82}); err != nil {
		return nil, fmt.Errorf("encode social card: %w", err)
	}
	return out.Bytes(), nil
}

func (b *builder) writeSocial(tag, title, subtitle string) (string, error) {
	p := socialPath(tag, title, subtitle)
	img, err := socialJPEG(title, subtitle, tag)
	if err != nil {
		return "", err
	}
	if err := b.write(p, img); err != nil {
		return "", err
	}
	return b.abs(p), nil
}

type socialJob struct{ tag, title, subtitle string }

func (b *builder) writeNodeSocialCards() error {
	jobs := make([]socialJob, 0, len(b.order))
	b.socialURLs = make(map[string]string, len(b.order))
	for _, v := range b.order {
		subtitle := v.Label()
		if subject := v.Subject(); subject != "" {
			subtitle += " · " + subject
		}
		jobs = append(jobs, socialJob{v.Tag, v.Name, subtitle})
		b.socialURLs[v.Tag] = b.abs(socialPath(v.Tag, v.Name, subtitle))
	}
	return b.writeSocialCards(jobs)
}

// JPEG encoding takes most of the time spent making cards. Write independent
// cards in parallel so a large content update does not delay deployment.
func (b *builder) writeSocialCards(jobs []socialJob) error {
	workers := min(runtime.GOMAXPROCS(0), 8)
	queue := make(chan socialJob, workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range queue {
				if _, err := b.writeSocial(j.tag, j.title, j.subtitle); err != nil {
					mu.Lock()
					if first == nil {
						first = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
	return first
}
