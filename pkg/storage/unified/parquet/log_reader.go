package parquet

import (
	"fmt"
	"strings"

	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
)

type LabelFilter struct {
	Key   string
	Value string
	Op    LabelFilterOp
}

type LabelFilterOp int

const (
	LabelFilterOpEqual LabelFilterOp = iota
	LabelFilterOpNotEqual
)

type LogLine struct {
	Timestamp int64
	Line      string
	Labels    map[string]string
}

type LogParquetReader struct {
	reader *file.Reader

	timestamp *int64Column
	line      *stringColumn
	labelKeys *stringColumn
	labelVals *stringColumn
	columns   []columnBuffer

	batchSize int64

	defLevels []int16
	repLevels []int16

	bufferSize  int
	bufferIndex int
	rowGroupIDX int

	current *LogLine
	err     error
	filters []LabelFilter
}

func NewLogParquetReader(inputPath string, batchSize int64, filters []LabelFilter) (*LogParquetReader, error) {
	rdr, err := openParquetFile(inputPath, false)
	if err != nil {
		return nil, err
	}

	schema := rdr.MetaData().Schema
	var initErr error

	makeStringCol := func(name string) *stringColumn {
		index := schema.ColumnIndexByName(name)
		if index < 0 {
			initErr = fmt.Errorf("missing column: %s", name)
		}
		return &stringColumn{
			index:  index,
			buffer: make([]parquet.ByteArray, batchSize),
		}
	}

	makeInt64Col := func(name string) *int64Column {
		index := schema.ColumnIndexByName(name)
		if index < 0 {
			initErr = fmt.Errorf("missing column: %s", name)
		}
		return &int64Column{
			index:  index,
			buffer: make([]int64, batchSize),
		}
	}

	reader := &LogParquetReader{
		reader: rdr,

		timestamp: makeInt64Col("timestamp"),
		line:      makeStringCol("line"),
		labelKeys: makeStringCol("label_key"),
		labelVals: makeStringCol("label_value"),

		batchSize: batchSize,
		defLevels: make([]int16, batchSize),
		repLevels: make([]int16, batchSize),
		filters:   filters,
	}

	if initErr != nil {
		_ = rdr.Close()
		return nil, initErr
	}

	reader.columns = []columnBuffer{
		reader.timestamp,
		reader.line,
		reader.labelKeys,
		reader.labelVals,
	}

	if rdr.NumRowGroups() < 1 {
		err = rdr.Close()
		reader.reader = nil
		return reader, err
	}

	err = reader.openRowGroup(rdr.RowGroup(0))
	if err != nil {
		_ = rdr.Close()
		return nil, err
	}

	err = reader.readBulk()
	if err != nil {
		_ = rdr.Close()
		return nil, err
	}

	return reader, nil
}

func (r *LogParquetReader) Next() bool {
	r.current = nil
	for r.err == nil && r.reader != nil {
		if r.bufferIndex >= r.bufferSize && r.line.reader.HasNext() {
			r.err = r.readBulk()
			if r.err != nil {
				r.close()
				return false
			}
		}

		if r.bufferSize > r.bufferIndex {
			i := r.bufferIndex
			r.bufferIndex++

			labels := parseLabels(
				r.labelKeys.buffer[i].String(),
				r.labelVals.buffer[i].String(),
			)

			if !r.matchesFilters(labels) {
				continue
			}

			r.current = &LogLine{
				Timestamp: r.timestamp.buffer[i],
				Line:      r.line.buffer[i].String(),
				Labels:    labels,
			}

			return true
		}

		r.rowGroupIDX++
		if r.rowGroupIDX >= r.reader.NumRowGroups() {
			r.close()
			return false
		}
		r.err = r.openRowGroup(r.reader.RowGroup(r.rowGroupIDX))
		if r.err != nil {
			r.close()
		}
	}

	return false
}

func (r *LogParquetReader) Line() *LogLine {
	return r.current
}

func (r *LogParquetReader) Err() error {
	return r.err
}

func (r *LogParquetReader) Close() error {
	r.close()
	return nil
}

func (r *LogParquetReader) close() {
	if r.reader != nil {
		_ = r.reader.Close()
		r.reader = nil
	}
}

func (r *LogParquetReader) openRowGroup(rgr *file.RowGroupReader) error {
	for _, c := range r.columns {
		err := c.open(rgr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *LogParquetReader) readBulk() error {
	r.bufferIndex = 0
	r.bufferSize = 0
	for i, c := range r.columns {
		count, err := c.batch(r.batchSize, r.defLevels, r.repLevels)
		if err != nil {
			return err
		}
		if i > 0 && r.bufferSize != count {
			return fmt.Errorf("expecting the same size for all columns")
		}
		r.bufferSize = count
	}
	return nil
}

func (r *LogParquetReader) matchesFilters(labels map[string]string) bool {
	if len(r.filters) == 0 {
		return true
	}

	for _, f := range r.filters {
		val, exists := labels[f.Key]

		switch f.Op {
		case LabelFilterOpEqual:
			if !exists || val != f.Value {
				return false
			}
		case LabelFilterOpNotEqual:
			if exists && val == f.Value {
				return false
			}
		}
	}

	return true
}

func parseLabels(keyStr, valStr string) map[string]string {
	labels := make(map[string]string)
	if keyStr == "" {
		return labels
	}

	keys := strings.Split(keyStr, "\x00")
	vals := strings.Split(valStr, "\x00")

	n := len(keys)
	if len(vals) < n {
		n = len(vals)
	}

	for i := 0; i < n; i++ {
		if keys[i] != "" {
			labels[keys[i]] = vals[i]
		}
	}

	return labels
}

type int64Column struct {
	index  int
	reader *file.Int64ColumnChunkReader
	buffer []int64
	count  int
}

func (c *int64Column) open(rgr *file.RowGroupReader) error {
	tmp, err := rgr.Column(c.index)
	if err != nil {
		return err
	}
	var ok bool
	c.reader, ok = tmp.(*file.Int64ColumnChunkReader)
	if !ok {
		return fmt.Errorf("expected int64 column")
	}
	return nil
}

func (c *int64Column) batch(batchSize int64, defLevels []int16, repLevels []int16) (int, error) {
	_, count, err := c.reader.ReadBatch(batchSize, c.buffer, defLevels, repLevels)
	c.count = count
	return count, err
}