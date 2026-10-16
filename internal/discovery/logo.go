package discovery

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"sort"
	"strings"

	"golang.org/x/image/draw"
)

type Logo struct {
	Source string `json:"source"`
	Data   string `json:"data"`
}

// FindLogo preserves the old single-application API. Workspaces with several
// applications use Inspect so callers can retain the choice of application.
func FindLogo(ctx context.Context, directory string) *Logo {
	result, err := Inspect(ctx, directory, InspectOptions{})
	if err != nil || len(result.Icons) != 1 {
		return nil
	}
	return &Logo{Source: result.Icons[0].Source, Data: result.Icons[0].Data}
}

// EncodeLogo validates a local image and returns the same bounded, inert image
// representation used by discovery. Callers must bound file reads separately.
func EncodeLogo(data []byte, extension string) string {
	if len(data) == 0 || len(data) > 2<<20 {
		return ""
	}
	return encodeLogo(data, strings.ToLower(extension))
}

func encodeLogo(data []byte, extension string) string {
	if extension == ".svg" {
		if len(data) > 128<<10 || !staticSVG(data) {
			return ""
		}
		return "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(data)
	}
	var img image.Image
	switch extension {
	case ".ico":
		img = decodeIcon(data)
	case ".icns":
		img = decodeICNS(data)
	}
	if img == nil {
		// Some web projects name a PNG favicon.ico. Validate the content's
		// raster header too, with the same pixel budget as ordinary images.
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 4<<20 {
			return ""
		}
		img, _, _ = image.Decode(bytes.NewReader(data))
	}
	if img == nil {
		return ""
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if max(w, h) > 128 {
		w, h = max(1, w*128/max(img.Bounds().Dx(), img.Bounds().Dy())), max(1, h*128/max(img.Bounds().Dx(), img.Bounds().Dy()))
	}
	thumb := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(thumb, thumb.Bounds(), img, img.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	if png.Encode(&out, thumb) != nil {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(out.Bytes())
}

// ICNS stores multi-resolution icons as 8-byte chunk headers followed by payload
// data. Modern macOS toolchains emit PNG payloads (icp4..ic14).
func decodeICNS(data []byte) image.Image {
	if len(data) < 16 || string(data[:4]) != "icns" {
		return nil
	}
	limit := int(binary.BigEndian.Uint32(data[4:8]))
	if limit < 16 || limit > len(data) {
		limit = len(data)
	}
	type frame struct {
		data []byte
		rank int
		area int
	}
	frames := []frame{}
	for offset := 8; offset+8 <= limit; {
		size := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
		if size < 8 || offset+size > limit {
			break
		}
		payload := data[offset+8 : offset+size]
		offset += size
		if !bytes.HasPrefix(payload, []byte("\x89PNG\r\n\x1a\n")) {
			continue
		}
		config, err := png.DecodeConfig(bytes.NewReader(payload))
		if err != nil || config.Width < 1 || config.Height < 1 || config.Width > 1024 || config.Height > 1024 {
			continue
		}
		rank := 1
		if config.Width >= 128 && config.Width <= 512 && config.Height >= 128 && config.Height <= 512 {
			rank = 2
		}
		frames = append(frames, frame{data: payload, rank: rank, area: config.Width * config.Height})
	}
	sort.SliceStable(frames, func(i, j int) bool {
		if frames[i].rank != frames[j].rank {
			return frames[i].rank > frames[j].rank
		}
		return frames[i].area > frames[j].area
	})
	for _, f := range frames {
		if img, err := png.Decode(bytes.NewReader(f.data)); err == nil {
			return img
		}
	}
	return nil
}

// ICO may contain PNG images or Windows DIB frames. The latter are still common
// in application icons and cannot be decoded by image.Decode on their own.
func decodeIcon(data []byte) image.Image {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		return nil
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count < 1 || count > 256 || len(data) < 6+16*count {
		return nil
	}
	type frame struct {
		data []byte
		area int
	}
	frames := []frame{}
	for i := 0; i < count; i++ {
		entry := data[6+i*16 : 6+(i+1)*16]
		length, offset := uint64(binary.LittleEndian.Uint32(entry[8:12])), uint64(binary.LittleEndian.Uint32(entry[12:16]))
		w, h := int(entry[0]), int(entry[1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		if offset >= uint64(6+16*count) && offset+length <= uint64(len(data)) && length > 8 {
			frames = append(frames, frame{data[offset : offset+length], w * h})
		}
	}
	sort.SliceStable(frames, func(i, j int) bool { return frames[i].area > frames[j].area })
	for _, frame := range frames {
		if bytes.HasPrefix(frame.data, []byte("\x89PNG\r\n\x1a\n")) {
			config, err := png.DecodeConfig(bytes.NewReader(frame.data))
			if err == nil && config.Width > 0 && config.Height > 0 && config.Width <= 256 && config.Height <= 256 {
				if img, err := png.Decode(bytes.NewReader(frame.data)); err == nil {
					return img
				}
			}
		} else if img := decodeIconDIB(frame.data); img != nil {
			return img
		}
	}
	return nil
}

func decodeIconDIB(data []byte) image.Image {
	if len(data) < 40 {
		return nil
	}
	header := int(binary.LittleEndian.Uint32(data[:4]))
	w, fullHeight := int(int32(binary.LittleEndian.Uint32(data[4:8]))), int(int32(binary.LittleEndian.Uint32(data[8:12])))
	bits := int(binary.LittleEndian.Uint16(data[14:16]))
	if header < 40 || header > len(data) || w < 1 || w > 256 || fullHeight < 2 || fullHeight > 512 || fullHeight%2 != 0 || binary.LittleEndian.Uint16(data[12:14]) != 1 || binary.LittleEndian.Uint32(data[16:20]) != 0 {
		return nil
	}
	if bits != 1 && bits != 4 && bits != 8 && bits != 24 && bits != 32 {
		return nil
	}
	h := fullHeight / 2
	paletteSize := 0
	if bits <= 8 {
		paletteSize = int(binary.LittleEndian.Uint32(data[32:36]))
		if paletteSize == 0 {
			paletteSize = 1 << bits
		}
		if paletteSize > 1<<bits {
			return nil
		}
	}
	pixels := header + paletteSize*4
	stride, maskStride := (w*bits+31)/32*4, (w+31)/32*4
	mask := pixels + stride*h
	if pixels > len(data) || mask > len(data) || (bits != 32 && mask+maskStride*h > len(data)) {
		return nil
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	hasAlpha := false
	for y := 0; y < h; y++ {
		row := data[pixels+(h-y-1)*stride:]
		for x := 0; x < w; x++ {
			c := color.NRGBA{A: 255}
			switch bits {
			case 24, 32:
				p := x * (bits / 8)
				c.B, c.G, c.R = row[p], row[p+1], row[p+2]
				if bits == 32 {
					c.A = row[p+3]
					hasAlpha = hasAlpha || c.A != 0
				}
			default:
				index := int(row[x*bits/8]>>(8-bits-(x*bits%8))) & ((1 << bits) - 1)
				if index >= paletteSize {
					return nil
				}
				p := header + index*4
				c.B, c.G, c.R = data[p], data[p+1], data[p+2]
			}
			img.SetNRGBA(x, y, c)
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := y*img.Stride + x*4
			if bits == 32 && !hasAlpha {
				img.Pix[p+3] = 255
			}
			if mask+maskStride*h <= len(data) && data[mask+(h-y-1)*maskStride+x/8]&(1<<uint(7-x%8)) != 0 {
				img.Pix[p+3] = 0
			}
		}
	}
	return img
}

func staticSVG(data []byte) bool {
	allowed := map[string]bool{}
	for _, name := range strings.Fields("svg g path rect circle ellipse line polygon polyline defs linearGradient radialGradient stop clipPath mask title desc filter feGaussianBlur feOffset feComposite feColorMatrix feBlend") {
		allowed[name] = true
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, nodes := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nodes > 0 && depth == 0
		}
		if err != nil {
			return false
		}
		switch value := token.(type) {
		case xml.Directive:
			return false
		case xml.ProcInst:
			if value.Target != "xml" {
				return false
			}
		case xml.StartElement:
			if !allowed[value.Name.Local] || (depth == 0 && (value.Name.Local != "svg" || nodes != 0)) || nodes >= 4096 || depth >= 64 {
				return false
			}
			depth++
			nodes++
			for _, a := range value.Attr {
				key, text := strings.ToLower(a.Name.Local), strings.TrimSpace(strings.ToLower(a.Value))
				if strings.HasPrefix(key, "on") || key == "style" || key == "base" {
					return false
				}
				if key == "href" && !strings.HasPrefix(text, "#") {
					return false
				}
				if strings.Contains(text, "url(") && (!strings.HasPrefix(text, "url(#") || !strings.HasSuffix(text, ")")) {
					return false
				}
			}
		case xml.EndElement:
			depth--
		}
	}
}
