package imageutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"time"
)

var ErrNoResizeNeeded = errors.New("no resize needed")

// ResizeImageFromURL downloads an image from url and returns a resized version
// if either dimension exceeds maxDim. The returned format string corresponds to
// the original image format (jpeg, png, etc.).
func ResizeImageFromURL(ctx context.Context, url string, maxDim int) (image.Image, string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024))
	if err != nil {
		return nil, "", err
	}
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= maxDim && h <= maxDim {
		return nil, "", ErrNoResizeNeeded
	}
	var newW, newH int
	if w > h {
		newW = maxDim
		newH = h * maxDim / w
		if newH < 1 {
			newH = 1
		}
	} else {
		newH = maxDim
		newW = w * maxDim / h
		if newW < 1 {
			newW = 1
		}
	}
	srcRGBA := toRGBA(img)
	dst := resizeBilinear(srcRGBA, newW, newH)
	return dst, format, nil
}

func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		b := rgba.Bounds()
		if b.Min.X == 0 && b.Min.Y == 0 {
			return rgba
		}
	}
	b := img.Bounds()
	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	return rgba
}

func resizeBilinear(src *image.RGBA, newW, newH int) *image.RGBA {
	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xRatio := float64(srcW-1) / float64(newW)
	yRatio := float64(srcH-1) / float64(newH)
	for y := 0; y < newH; y++ {
		for x := 0; x < newW; x++ {
			sx := xRatio * float64(x)
			sy := yRatio * float64(y)
			x0 := int(sx)
			y0 := int(sy)
			x1 := x0 + 1
			y1 := y0 + 1
			if x1 >= srcW {
				x1 = srcW - 1
			}
			if y1 >= srcH {
				y1 = srcH - 1
			}
			dx := sx - float64(x0)
			dy := sy - float64(y0)
			i00 := (y0*srcW + x0) * 4
			i10 := (y0*srcW + x1) * 4
			i01 := (y1*srcW + x0) * 4
			i11 := (y1*srcW + x1) * 4
			pix := src.Pix
			di := (y*newW + x) * 4
			for c := 0; c < 4; c++ {
				v := (1.0-dx)*(1.0-dy)*float64(pix[i00+c]) +
					dx*(1.0-dy)*float64(pix[i10+c]) +
					(1.0-dx)*dy*float64(pix[i01+c]) +
					dx*dy*float64(pix[i11+c])
				dst.Pix[di+c] = uint8(v + 0.5)
			}
		}
	}
	return dst
}

// ResizeImageBytes decodes the provided image bytes and resizes it if either
// dimension exceeds maxDim. Returns the resized image and detected format.
func ResizeImageBytes(data []byte, maxDim int) (image.Image, string, error) {
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= maxDim && h <= maxDim {
		return nil, "", ErrNoResizeNeeded
	}
	var newW, newH int
	if w > h {
		newW = maxDim
		newH = h * maxDim / w
		if newH < 1 {
			newH = 1
		}
	} else {
		newH = maxDim
		newW = w * maxDim / h
		if newW < 1 {
			newW = 1
		}
	}
	srcRGBA := toRGBA(img)
	dst := resizeBilinear(srcRGBA, newW, newH)
	return dst, format, nil
}

// EncodeImage encodes the image to the requested format.
// If format is not "png" it falls back to JPEG.
func EncodeImage(img image.Image, format string, quality int) ([]byte, string, error) {
	var buf bytes.Buffer
	var contentType string
	switch format {
	case "png":
		contentType = "image/png"
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", err
		}
	default:
		contentType = "image/jpeg"
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, "", err
		}
	}
	return buf.Bytes(), contentType, nil
}
