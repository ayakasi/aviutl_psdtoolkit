package img

import (
	"context"
	"image"
	"time"

	"github.com/disintegration/gift"
	"github.com/pkg/errors"

	"github.com/oov/aviutl_psdtoolkit/src/go/warn"
	"github.com/oov/downscale"
	"github.com/oov/psd/composite"
)

type Flip int

const (
	FlipNone Flip = iota
	FlipX
	FlipY
	FlipXY
)

type Toucher interface {
	Touch()
	LastAccess() time.Time
}

type Image struct {
	FilePath *string
	FileHash uint32
	Toucher  Toucher

	PSD    *composite.Tree
	image  *image.NRGBA
	Layers *LayerManager
	Flip   Flip

	InitialLayerState *string

	Modified bool

	Scale   float32
	OffsetX int
	OffsetY int

	PFV *PFV
}

func (img *Image) Touch() {
	img.Toucher.Touch()
}

func (img *Image) LastAccess() time.Time {
	return img.Toucher.LastAccess()
}

func (img *Image) Clone() *Image {
	r := *img
	r.image = nil
	return &r
}

func (img *Image) FlipX() bool {
	return img.Flip == FlipX || img.Flip == FlipXY
}

func (img *Image) FlipY() bool {
	return img.Flip == FlipY || img.Flip == FlipXY
}

func (img *Image) SetFlipX(v bool) bool {
	if (img.Flip&FlipX != 0) == v {
		return false
	}
	if v {
		img.Flip |= FlipX
	} else {
		img.Flip &= ^FlipX
	}
	return true
}

func (img *Image) SetFlipY(v bool) bool {
	if (img.Flip&FlipY != 0) == v {
		return false
	}
	if v {
		img.Flip |= FlipY
	} else {
		img.Flip &= ^FlipY
	}
	return true
}

func (img *Image) ScaledCanvasRect() image.Rectangle {
	r := img.PSD.CanvasRect
	r.Max.X = r.Min.X + int(float32(r.Dx())*img.Scale+0.5)
	r.Max.Y = r.Min.Y + int(float32(r.Dy())*img.Scale+0.5)
	if r.Dx() < 1 {
		r.Max.X = r.Min.X + 1
	}
	if r.Dy() < 1 {
		r.Max.Y = r.Min.Y + 1
	}
	return r
}

func (img *Image) Render(ctx context.Context) (*image.NRGBA, error) {
	var err error
	if img.image == nil {
		img.image = image.NewNRGBA(img.PSD.CanvasRect)
		err = img.PSD.Renderer.Render(ctx, img.image)
	} else {
		err = img.PSD.Renderer.RenderDiff(ctx, img.image)
	}
	if err != nil {
		return nil, errors.Wrap(err, "img: render failed")
	}
	img.Modified = false
	nrgba := img.image
	if img.Scale < 1 {
		tmp := image.NewNRGBA(img.ScaledCanvasRect())
		if err = downscale.NRGBAGamma(ctx, tmp, nrgba, 2.2); err != nil {
			return nil, errors.Wrap(err, "img: downscale failed")
		}
		nrgba = tmp
	}
	f := img.Flip
	if f != FlipNone {
		tmp := image.NewNRGBA(nrgba.Rect)
		g := gift.New()
		if f == FlipX || f == FlipXY {
			g.Add(gift.FlipHorizontal())
		}
		if f == FlipY || f == FlipXY {
			g.Add(gift.FlipVertical())
		}
		g.Draw(tmp, nrgba)
		nrgba = tmp
	}
	return nrgba, nil
}

func (img *Image) Serialize() (string, error) {
	s, err := img.Layers.Serialize()
	if err != nil {
		return "", errors.Wrap(err, "Image.Serialize: failed to serialize")
	}
	return "L." + itoa(int(img.Flip)) + " " + s, nil
}

func (img *Image) Deserialize(s string) (bool, error) {
	m, f, err := img.Layers.Deserialize(s, img.Flip, img.PFV)
	if err != nil {
		return false, err
	}
	img.Flip = f
	return m, nil
}

// TODO: faview selected item state
type ProjectState struct {
	Version  int
	FilePath string
	Flip     Flip
	Layer    map[string]SerializedData
	PFV      PFVSerializedData
}

func (img *Image) SerializeProject() *ProjectState {
	return &ProjectState{
		Version:  1,
		FilePath: *img.FilePath,
		Flip:     img.Flip,
		Layer:    img.Layers.SerializeSafe(),
		PFV:      img.PFV.Serialize(),
	}
}

func (img *Image) DeserializeProject(state *ProjectState) (warn.Warning, error) {
	var wr warn.Warning
	img.Flip = state.Flip
	if w, err := img.Layers.DeserializeSafe(state.Layer); err != nil {
		return wr, errors.Wrap(err, "Image.DeserializeProject: failed to deserialize")
	} else if w != nil {
		wr = append(wr, w...)
	}
	if w, err := img.PFV.Deserialize(state.PFV); err != nil {
		return wr, errors.Wrap(err, "Image.DeserializeProject: failed to deserialize")
	} else if w != nil {
		wr = append(wr, w...)
	}
	return wr, nil
}
