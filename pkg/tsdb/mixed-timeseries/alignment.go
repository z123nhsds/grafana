package mixedtimeseries

import (
	"math"
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

// AlignFrames 对齐多个时间序列帧到共享时间线上，使用双指针算法（O(n+m)复杂度）
func AlignFrames(frames []*data.Frame, step time.Duration) []*data.Frame {
	if len(frames) == 0 {
		return nil
	}

	// 首先收集所有唯一的对齐时间点
	alignedTimestamps := collectAlignedTimestamps(frames, step)
	if len(alignedTimestamps) == 0 {
		return nil
	}

	// 为每个原始帧创建对齐后的新帧
	result := make([]*data.Frame, 0, len(frames))
	for _, frame := range frames {
		alignedFrame := alignSingleFrame(frame, alignedTimestamps, step)
		if alignedFrame != nil {
			result = append(result, alignedFrame)
		}
	}

	return result
}

// collectAlignedTimestamps 收集所有需要对齐到的时间点
func collectAlignedTimestamps(frames []*data.Frame, step time.Duration) []time.Time {
	if len(frames) == 0 {
		return nil
	}

	// 使用 map 收集唯一的时间点（对齐后）
	timestampSet := make(map[int64]bool)

	for _, frame := range frames {
		if frame == nil || len(frame.Fields) == 0 {
			continue
		}

		// 找到时间字段
		timeFieldIdx := -1
		for i, field := range frame.Fields {
			if field.Type() == data.FieldTypeTime {
				timeFieldIdx = i
				break
			}
		}
		if timeFieldIdx == -1 {
			continue
		}

		// 收集并对齐该帧的所有时间点
		timeField := frame.Fields[timeFieldIdx]
		for i := 0; i < timeField.Len(); i++ {
			t, ok := timeField.At(i).(time.Time)
			if !ok {
				continue
			}
			alignedTs := alignTimestamp(t, step)
			timestampSet[alignedTs.UnixMilli()] = true
		}
	}

	// 转换为有序的时间戳切片
	result := make([]time.Time, 0, len(timestampSet))
	for tsMs := range timestampSet {
		result = append(result, time.UnixMilli(tsMs).UTC())
	}

	// 排序，使用内置的 sort 包，时间复杂度为 O(n log n)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Before(result[j])
	})

	return result
}

// alignTimestamp 将时间戳对齐到步长网格
func alignTimestamp(t time.Time, step time.Duration) time.Time {
	if step <= 0 {
		return t
	}
	tsMs := t.UnixMilli()
	stepMs := step.Milliseconds()
	// 四舍五入到最近的步长倍数
	alignedMs := int64(math.Round(float64(tsMs)/float64(stepMs))) * stepMs
	return time.UnixMilli(alignedMs).UTC()
}

// alignSingleFrame 将单个帧对齐到指定的时间点列表
func alignSingleFrame(frame *data.Frame, targetTimestamps []time.Time, step time.Duration) *data.Frame {
	if frame == nil || len(frame.Fields) == 0 || len(targetTimestamps) == 0 {
		return nil
	}

	// 找到时间字段
	timeFieldIdx := -1
	var timeField *data.Field
	for i, field := range frame.Fields {
		if field.Type() == data.FieldTypeTime {
			timeFieldIdx = i
			timeField = field
			break
		}
	}
	if timeFieldIdx == -1 {
		return nil
	}

	// 收集原始数据（对齐后的时间戳和对应索引）
	type alignedPoint struct {
		tsMs  int64
		index int
	}
	var alignedPoints []alignedPoint
	for i := 0; i < timeField.Len(); i++ {
		t, ok := timeField.At(i).(time.Time)
		if !ok {
			continue
		}
		alignedTsMs := alignTimestamp(t, step).UnixMilli()
		alignedPoints = append(alignedPoints, alignedPoint{
			tsMs:  alignedTsMs,
			index: i,
		})
	}

	// 创建结果字段
	resultFields := make([]*data.Field, 0, len(frame.Fields))

	// 添加对齐后的时间字段
	alignedTimes := make([]time.Time, len(targetTimestamps))
	copy(alignedTimes, targetTimestamps)
	resultFields = append(resultFields, data.NewField(timeField.Name, timeField.Labels, alignedTimes))

	// 对其他每个字段进行对齐
	for i, field := range frame.Fields {
		if i == timeFieldIdx {
			continue // 跳过时间字段，已经处理过了
		}

		// 创建对齐后的新字段
		newField := data.NewFieldFromFieldType(field.Type(), len(targetTimestamps))
		newField.Name = field.Name
		newField.Labels = field.Labels.Copy()
		if field.Config != nil {
			newField.Config = field.Config.Copy()
		}

		// 使用双指针方法填充新字段，O(n + m) 复杂度
		j := 0 // 原始数据的指针
		for k, targetTs := range targetTimestamps {
			targetTsMs := targetTs.UnixMilli()

			// 移动指针直到找到匹配或超过目标
			for j < len(alignedPoints) && alignedPoints[j].tsMs < targetTsMs {
				j++
			}

			// 检查是否找到匹配项
			if j < len(alignedPoints) && alignedPoints[j].tsMs == targetTsMs {
				// 找到匹配，使用该值
				newField.Set(k, field.At(alignedPoints[j].index))
			}
			// 没有找到匹配，保持零值
		}

		resultFields = append(resultFields, newField)
	}

	// 创建新的对齐帧
	result := data.NewFrame(frame.Name, resultFields...)
	result.RefID = frame.RefID
	if frame.Meta != nil {
		result.Meta = frame.Meta.Copy()
	}

	return result
}

