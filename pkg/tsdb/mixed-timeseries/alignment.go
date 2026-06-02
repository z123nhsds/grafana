package mixedtimeseries

import (
	"errors"
	"fmt"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

var (
	ErrNoTimeField       = errors.New("frame has no time field")
	ErrNilFrame          = errors.New("frame is nil")
	ErrStepZero          = errors.New("step must be positive")
	ErrFrameCount        = errors.New("not enough frames provided")
	ErrTimeFieldMismatch = errors.New("time field type mismatch across frames")
)

func extractTimeField(frame *data.Frame) (*data.Field, error) {
	if frame == nil {
		return nil, ErrNilFrame
	}
	for _, f := range frame.Fields {
		if f.Type() == data.FieldTypeTime {
			return f, nil
		}
	}
	return nil, ErrNoTimeField
}

func timeAt(field *data.Field, idx int) time.Time {
	return field.At(idx).(time.Time)
}

type alignedPair struct {
	refIdx int
	tgtIdx int
}

func alignTimeFields(ref, tgt *data.Field, step time.Duration) []alignedPair {
	halfStep := step / 2
	var pairs []alignedPair

	i, j := 0, 0
	refLen := ref.Len()
	tgtLen := tgt.Len()

	for i < refLen && j < tgtLen {
		refT := timeAt(ref, i)
		tgtT := timeAt(tgt, j)
		diff := tgtT.Sub(refT)

		if diff < -halfStep {
			j++
			continue
		}
		if diff > halfStep {
			i++
			continue
		}
		pairs = append(pairs, alignedPair{refIdx: i, tgtIdx: j})
		i++
		j++
	}

	return pairs
}

func buildAlignedFrame(frame *data.Frame, pairs []alignedPair, refTimeField *data.Field) *data.Frame {
	newFrame := data.NewFrame(frame.Name)

	newTimeField := data.NewFieldFromFieldType(data.FieldTypeTime, len(pairs))
	newTimeField.Name = refTimeField.Name
	newTimeField.Labels = refTimeField.Labels
	if refTimeField.Config != nil {
		newTimeField.Config = refTimeField.Config
	}
	for i, p := range pairs {
		newTimeField.Set(i, timeAt(refTimeField, p.refIdx))
	}
	newFrame.Fields = append(newFrame.Fields, newTimeField)

	for _, f := range frame.Fields {
		if f.Type() == data.FieldTypeTime {
			continue
		}
		newField := data.NewFieldFromFieldType(f.Type(), len(pairs))
		newField.Name = f.Name
		newField.Labels = f.Labels
		if f.Config != nil {
			newField.Config = f.Config
		}
		for i, p := range pairs {
			newField.Set(i, f.CopyAt(p.tgtIdx))
		}
		newFrame.Fields = append(newFrame.Fields, newField)
	}

	if frame.Meta != nil {
		newFrame.Meta = frame.Meta
	}

	return newFrame
}

// AlignToReference aligns each target frame's timestamps to the reference frame's
// time grid using a two-pointer algorithm with ±step/2 tolerance. Each returned
// frame contains only the rows whose timestamps matched the reference within the
// tolerance window.
func AlignToReference(ref *data.Frame, targets []*data.Frame, step time.Duration) ([]*data.Frame, error) {
	if step <= 0 {
		return nil, ErrStepZero
	}
	if ref == nil {
		return nil, ErrNilFrame
	}
	if len(targets) == 0 {
		return nil, ErrFrameCount
	}

	refTimeField, err := extractTimeField(ref)
	if err != nil {
		return nil, fmt.Errorf("reference frame: %w", err)
	}

	result := make([]*data.Frame, 0, len(targets))
	for idx, tgt := range targets {
		if tgt == nil {
			return nil, fmt.Errorf("target frame %d: %w", idx, ErrNilFrame)
		}

		tgtTimeField, err := extractTimeField(tgt)
		if err != nil {
			return nil, fmt.Errorf("target frame %d: %w", idx, err)
		}

		if tgtTimeField.Type() != refTimeField.Type() {
			return nil, fmt.Errorf("target frame %d: %w", idx, ErrTimeFieldMismatch)
		}

		pairs := alignTimeFields(refTimeField, tgtTimeField, step)
		result = append(result, buildAlignedFrame(tgt, pairs, refTimeField))
	}

	return result, nil
}

// MergeFrames combines multiple time series frames into a single wide frame.
// The first frame provides the reference time grid; subsequent frames are
// aligned to it with ±step/2 tolerance. Non-matching rows leave nil values.
func MergeFrames(frames []*data.Frame, step time.Duration) (*data.Frame, error) {
	if step <= 0 {
		return nil, ErrStepZero
	}
	if len(frames) == 0 {
		return nil, ErrFrameCount
	}
	if len(frames) == 1 {
		if frames[0] == nil {
			return nil, ErrNilFrame
		}
		_, err := extractTimeField(frames[0])
		if err != nil {
			return nil, err
		}
		return frames[0], nil
	}

	ref := frames[0]
	refTimeField, err := extractTimeField(ref)
	if err != nil {
		return nil, fmt.Errorf("reference frame: %w", err)
	}

	mergedTimeField := data.NewFieldFromFieldType(data.FieldTypeTime, refTimeField.Len())
	mergedTimeField.Name = refTimeField.Name
	mergedTimeField.Labels = refTimeField.Labels
	if refTimeField.Config != nil {
		mergedTimeField.Config = refTimeField.Config
	}
	for i := 0; i < refTimeField.Len(); i++ {
		mergedTimeField.Set(i, timeAt(refTimeField, i))
	}

	result := data.NewFrame("")
	result.Fields = append(result.Fields, mergedTimeField)

	for _, f := range ref.Fields {
		if f.Type() == data.FieldTypeTime {
			continue
		}
		newField := data.NewFieldFromFieldType(f.Type(), f.Len())
		newField.Name = f.Name
		newField.Labels = f.Labels
		if f.Config != nil {
			newField.Config = f.Config
		}
		for i := 0; i < f.Len(); i++ {
			newField.Set(i, f.CopyAt(i))
		}
		result.Fields = append(result.Fields, newField)
	}

	for tgtIdx := 1; tgtIdx < len(frames); tgtIdx++ {
		tgt := frames[tgtIdx]
		if tgt == nil {
			return nil, fmt.Errorf("frame %d: %w", tgtIdx, ErrNilFrame)
		}

		tgtTimeField, err := extractTimeField(tgt)
		if err != nil {
			return nil, fmt.Errorf("frame %d: %w", tgtIdx, err)
		}

		pairs := alignTimeFields(refTimeField, tgtTimeField, step)

		for _, f := range tgt.Fields {
			if f.Type() == data.FieldTypeTime {
				continue
			}

			newField := data.NewFieldFromFieldType(f.Type(), mergedTimeField.Len())
			newField.Name = f.Name
			newField.Labels = f.Labels
			if f.Config != nil {
				newField.Config = f.Config
			}

			pairMap := make(map[int]int, len(pairs))
			for _, p := range pairs {
				pairMap[p.refIdx] = p.tgtIdx
			}

			for i := 0; i < mergedTimeField.Len(); i++ {
				if mapped, ok := pairMap[i]; ok {
					newField.Set(i, f.CopyAt(mapped))
				}
			}

			result.Fields = append(result.Fields, newField)
		}
	}

	return result, nil
}
