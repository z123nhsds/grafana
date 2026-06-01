package bulkparquet

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/grafana/grafana/pkg/storage/unified/resourcepb"
)

type BulkRequestIterator interface {
	Next() bool
	Request() *resourcepb.BulkRequest
	RollbackRequested() bool
}

type LabelFilter struct {
	Key   string
	Value string
	Op    LabelOperator
}

type LabelOperator int

const (
	LabelOpEqual LabelOperator = iota
	LabelOpNotEqual
	LabelOpRegexMatch
	LabelOpRegexNotMatch
)

func (op LabelOperator) String() string {
	switch op {
	case LabelOpEqual:
		return "="
	case LabelOpNotEqual:
		return "!="
	case LabelOpRegexMatch:
		return "=~"
	case LabelOpRegexNotMatch:
		return "!=~"
	default:
		return "="
	}
}

type ParquetReaderConfig struct {
	InputPath  string
	BatchSize  int64
	LabelFilters []LabelFilter
	MixedGroup  string
	MixedResource string
}

type parquetReader struct {
	reader *file.Reader

	namespace *byteArrayColumn
	group     *byteArrayColumn
	resource  *byteArrayColumn
	name      *byteArrayColumn
	value     *byteArrayColumn
	folder    *byteArrayColumn
	action    *int32Column
	timestamp *int64Column
	labels    *byteArrayColumn
	columns   []columnBuffer

	batchSize int64

	defLevels []int16
	repLevels []int16

	bufferSize   int
	bufferIndex  int
	rowGroupIdx  int

	req         *resourcepb.BulkRequest
	err         error
	labelFilters []LabelFilter
	namespaceVal string
	groupVal     string
	resourceVal  string
}

var _ BulkRequestIterator = (*parquetReader)(nil)

func NewParquetReader(ctx context.Context, cfg ParquetReaderConfig) (BulkRequestIterator, error) {
	return newParquetReader(ctx, cfg)
}

func newParquetReader(ctx context.Context, cfg ParquetReaderConfig) (*parquetReader, error) {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1024
	}

	rdr, err := file.OpenParquetFile(cfg.InputPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to open parquet file: %w", err)
	}

	schema := rdr.Metadata().Schema

	makeByteArrayColumn := func(name string) *byteArrayColumn {
		index := schema.ColumnIndexByName(name)
		if index < 0 {
			return &byteArrayColumn{index: -1}
		}
		return &byteArrayColumn{
			index:  index,
			buffer: make([]parquet.ByteArray, cfg.BatchSize),
		}
	}

	reader := &parquetReader{
		reader: rdr,

		namespace: makeByteArrayColumn("namespace"),
		group:     makeByteArrayColumn("group"),
		resource:  makeByteArrayColumn("resource"),
		name:      makeByteArrayColumn("name"),
		value:     makeByteArrayColumn("value"),
		folder:    makeByteArrayColumn("folder"),
		timestamp: &int64Column{
			index:  schema.ColumnIndexByName("timestamp"),
			buffer: make([]int64, cfg.BatchSize),
		},
		action: &int32Column{
			index:  schema.ColumnIndexByName("action"),
			buffer: make([]int32, cfg.BatchSize),
		},
		labels: makeByteArrayColumn("labels"),

		batchSize: cfg.BatchSize,
		defLevels: make([]int16, cfg.BatchSize),
		repLevels: make([]int16, cfg.BatchSize),

		labelFilters: cfg.LabelFilters,
	}

	if reader.namespace.index < 0 {
		_ = rdr.Close()
		return nil, fmt.Errorf("missing required column: namespace")
	}

	reader.columns = []columnBuffer{
		reader.namespace,
		reader.group,
		reader.resource,
		reader.name,
		reader.folder,
		reader.action,
		reader.timestamp,
		reader.value,
		reader.labels,
	}

	if rdr.NumRowGroups() < 1 {
		_ = rdr.Close()
		reader.reader = nil
		return reader, nil
	}

	err = reader.openRowGroup(rdr.RowGroup(0))
	if err != nil {
		_ = rdr.Close()
		return nil, fmt.Errorf("failed to open first row group: %w", err)
	}

	err = reader.readBulk()
	if err != nil {
		_ = rdr.Close()
		return nil, fmt.Errorf("failed to read first batch: %w", err)
	}

	return reader, nil
}

func (r *parquetReader) openRowGroup(rgr *file.RowGroupReader) error {
	for _, c := range r.columns {
		if c == nil {
			continue
		}
		if err := c.open(rgr); err != nil {
			return err
		}
	}
	return nil
}

func (r *parquetReader) readBulk() error {
	r.bufferIndex = 0
	r.bufferSize = 0

	for i, c := range r.columns {
		if c == nil {
			continue
		}
		count, err := c.batch(r.batchSize, r.defLevels, r.repLevels)
		if err != nil {
			return err
		}
		if i > 0 && r.bufferSize != count && count > 0 {
			return fmt.Errorf("column batch size mismatch")
		}
		r.bufferSize = count
	}

	return nil
}

func (r *parquetReader) matchesLabelFilters(labelsStr string) bool {
	if len(r.labelFilters) == 0 {
		return true
	}

	if labelsStr == "" {
		return false
	}

	for _, filter := range r.labelFilters {
		if !matchLabel(labelsStr, filter) {
			return false
		}
	}
	return true
}

func matchLabel(labelsStr string, filter LabelFilter) bool {
	key := filter.Key + "=\""
	startIdx := 0
	for {
		idx := findLabelValue(labelsStr, key, startIdx)
		if idx < 0 {
			return filter.Op == LabelOpNotEqual || filter.Op == LabelOpRegexNotMatch
		}

		valueStart := idx + len(key)
		valueEnd := findLabelEnd(labelsStr, valueStart)
		if valueEnd < 0 {
			valueEnd = len(labelsStr)
		}

		value := labelsStr[valueStart:valueEnd]

		switch filter.Op {
		case LabelOpEqual:
			if value == filter.Value {
				return true
			}
		case LabelOpNotEqual:
			if value != filter.Value {
				return true
			}
		case LabelOpRegexMatch:
			if matchRegex(value, filter.Value) {
				return true
			}
		case LabelOpRegexNotMatch:
			if !matchRegex(value, filter.Value) {
				return true
			}
		}

		startIdx = valueEnd
	}

	return false
}

func findLabelValue(labelsStr, key string, startIdx int) int {
	for i := startIdx; i <= len(labelsStr)-len(key); i++ {
		if labelsStr[i:i+len(key)] == key {
			return i
		}
	}
	return -1
}

func findLabelEnd(labelsStr string, start int) int {
	for i := start; i < len(labelsStr); i++ {
		if labelsStr[i] == '"' {
			return i
		}
		if labelsStr[i] == '\\' && i+1 < len(labelsStr) {
			i++
		}
	}
	return -1
}

func matchRegex(value, pattern string) bool {
	matched, _ := matchRegexSimple(value, pattern)
	return matched
}

func matchRegexSimple(value, pattern string) (bool, error) {
	if pattern == "" {
		return value == "", nil
	}

	isRegex := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c == '.' || c == '*' || c == '+' || c == '?' || c == '[' || c == '(' || c == ')' || c == '{' || c == '}' || c == '\\' || c == '^' || c == '$' || c == '|' {
			isRegex = true
			break
		}
	}

	if !isRegex {
		return value == pattern, nil
	}

	start := 0
	end := len(value)
	if len(pattern) >= 2 && pattern[0] == '^' {
		pattern = pattern[1:]
	} else {
		start = 0
	}
	if len(pattern) >= 2 && pattern[len(pattern)-1] == '$' {
		pattern = pattern[:len(pattern)-1]
	} else {
		end = len(value)
	}

	patternIdx := 0
	valueIdx := start

	for valueIdx < end && patternIdx < len(pattern) {
		p := pattern[patternIdx]
		switch p {
		case '.':
			patternIdx++
			valueIdx++
			if patternIdx < len(pattern) && pattern[patternIdx] == '*' {
				patternIdx++
				for valueIdx < end {
					if matchRegexSimple(value[valueIdx:end], pattern[patternIdx:]) {
						return true, nil
					}
					valueIdx++
				}
			} else if patternIdx < len(pattern) && pattern[patternIdx] == '+' {
				patternIdx++
				valueIdx++
			}
		case '\\':
			if patternIdx+1 < len(pattern) {
				patternIdx++
				p = pattern[patternIdx]
				if valueIdx < end && value[valueIdx] == p {
					patternIdx++
					valueIdx++
				} else {
					return false, nil
				}
			}
		case '*':
			patternIdx++
			for valueIdx <= end {
				if matchRegexSimple(value[valueIdx:end], pattern[patternIdx:]) {
					return true, nil
				}
				valueIdx++
			}
			return false, nil
		case '+':
			patternIdx++
			if valueIdx >= end {
				return false, nil
			}
			valueIdx++
		case '?':
			patternIdx++
			if valueIdx < end {
				valueIdx++
			}
		default:
			if valueIdx < end && value[valueIdx] == p {
				patternIdx++
				valueIdx++
			} else {
				return false, nil
			}
		}
	}

	if patternIdx >= len(pattern) && valueIdx >= end {
		return true, nil
	}
	if patternIdx >= len(pattern) && start > 0 && len(pattern) > 0 && pattern[len(pattern)-1] == '$' {
		return valueIdx >= end, nil
	}

	return patternIdx >= len(pattern) && valueIdx >= end, nil
}

func (r *parquetReader) Next() bool {
	r.req = nil

	for r.err == nil && r.reader != nil {
		if r.bufferIndex >= r.bufferSize {
			if !r.hasNextRowGroup() {
				r.close()
				return false
			}

			r.rowGroupIdx++
			if r.rowGroupIdx >= r.reader.NumRowGroups() {
				r.close()
				return false
			}

			r.err = r.openRowGroup(r.reader.RowGroup(r.rowGroupIdx))
			if r.err != nil {
				r.close()
				return false
			}

			r.err = r.readBulk()
			if r.err != nil {
				r.close()
				return false
			}
		}

		for r.bufferIndex < r.bufferSize {
			i := r.bufferIndex

			namespaceStr := r.namespace.buffer[i].String()
			groupStr := r.group.buffer[i].String()
			resourceStr := r.resource.buffer[i].String()
			labelsStr := r.labels.buffer[i].String()

			if !r.matchesLabelFilters(labelsStr) {
				r.bufferIndex++
				continue
			}

			r.bufferIndex++

			r.req = &resourcepb.BulkRequest{
				Key: &resourcepb.ResourceKey{
					Group:     groupStr,
					Resource:  resourceStr,
					Namespace: namespaceStr,
					Name:      r.name.buffer[i].String(),
				},
				Action: resourcepb.BulkRequest_Action(r.action.buffer[i]),
				Value:  r.value.buffer[i].Bytes(),
				Folder: r.folder.buffer[i].String(),
			}

			return true
		}
	}

	return false
}

func (r *parquetReader) hasNextRowGroup() bool {
	if r.reader == nil {
		return false
	}
	return r.rowGroupIdx+1 < r.reader.NumRowGroups()
}

func (r *parquetReader) Request() *resourcepb.BulkRequest {
	return r.req
}

func (r *parquetReader) RollbackRequested() bool {
	return r.err != nil
}

func (r *parquetReader) close() {
	if r.reader != nil {
		_ = r.reader.Close()
		r.reader = nil
	}
}

type columnBuffer interface {
	open(rgr *file.RowGroupReader) error
	batch(batchSize int64, defLevels []int16, repLevels []int16) (int, error)
}

type byteArrayColumn struct {
	index  int
	reader *file.ByteArrayColumnChunkReader
	buffer []parquet.ByteArray
	count  int
}

func (c *byteArrayColumn) open(rgr *file.RowGroupReader) error {
	if c.index < 0 {
		return nil
	}
	tmp, err := rgr.Column(c.index)
	if err != nil {
		return err
	}
	var ok bool
	c.reader, ok = tmp.(*file.ByteArrayColumnChunkReader)
	if !ok {
		return fmt.Errorf("expected byte array column reader")
	}
	return nil
}

func (c *byteArrayColumn) batch(batchSize int64, defLevels []int16, repLevels []int16) (int, error) {
	if c.reader == nil {
		return 0, nil
	}
	_, count, err := c.reader.ReadBatch(batchSize, c.buffer, defLevels, repLevels)
	c.count = count
	return count, err
}

type int32Column struct {
	index  int
	reader *file.Int32ColumnChunkReader
	buffer []int32
	count  int
}

func (c *int32Column) open(rgr *file.RowGroupReader) error {
	if c.index < 0 {
		return nil
	}
	tmp, err := rgr.Column(c.index)
	if err != nil {
		return err
	}
	var ok bool
	c.reader, ok = tmp.(*file.Int32ColumnChunkReader)
	if !ok {
		return fmt.Errorf("expected int32 column reader")
	}
	return nil
}

func (c *int32Column) batch(batchSize int64, defLevels []int16, repLevels []int16) (int, error) {
	if c.reader == nil {
		return 0, nil
	}
	_, count, err := c.reader.ReadBatch(batchSize, c.buffer, defLevels, repLevels)
	c.count = count
	return count, err
}

type int64Column struct {
	index  int
	reader *file.Int64ColumnChunkReader
	buffer []int64
	count  int
}

func (c *int64Column) open(rgr *file.RowGroupReader) error {
	if c.index < 0 {
		return nil
	}
	tmp, err := rgr.Column(c.index)
	if err != nil {
		return err
	}
	var ok bool
	c.reader, ok = tmp.(*file.Int64ColumnChunkReader)
	if !ok {
		return fmt.Errorf("expected int64 column reader")
	}
	return nil
}

func (c *int64Column) batch(batchSize int64, defLevels []int16, repLevels []int16) (int, error) {
	if c.reader == nil {
		return 0, nil
	}
	_, count, err := c.reader.ReadBatch(batchSize, c.buffer, defLevels, repLevels)
	c.count = count
	return count, err
}

type BulkParquetWriter struct {
	file       *os.File
	writer     *file.Writer
	rowGroupSize int64
	rowsInGroup  int
	config      ParquetReaderConfig
}

type WriteOptions struct {
	RowGroupSize int64
	Compression  parquet.CompressionCodec
}

func NewWriter(outputPath string, opts WriteOptions) (*BulkParquetWriter, error) {
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	if opts.RowGroupSize <= 0 {
		opts.RowGroupSize = 10000
	}
	if opts.Compression == 0 {
		opts.Compression = parquet.CompressionSnappy
	}

	schema := parquet.Schema{
		Name: "bulk_request",
		Fields: []parquet.Field{
			parquet.LeafField{Name: "namespace", Type: parquet.ByteArrayType, Compression: opts.Compression},
			parquet.LeafField{Name: "group", Type: parquet.ByteArrayType, Compression: opts.Compression},
			parquet.LeafField{Name: "resource", Type: parquet.ByteArrayType, Compression: opts.Compression},
			parquet.LeafField{Name: "name", Type: parquet.ByteArrayType, Compression: opts.Compression},
			parquet.LeafField{Name: "value", Type: parquet.ByteArrayType, Compression: opts.Compression},
			parquet.LeafField{Name: "folder", Type: parquet.ByteArrayType, Compression: opts.Compression},
			parquet.LeafField{Name: "action", Type: parquet.Int32Type, Compression: opts.Compression},
			parquet.LeafField{Name: "timestamp", Type: parquet.Int64Type, Compression: opts.Compression},
			parquet.LeafField{Name: "labels", Type: parquet.ByteArrayType, Compression: opts.Compression},
		},
	}

	writer, err := file.NewWriter(schema, int(opts.RowGroupSize))
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("failed to create parquet writer: %w", err)
	}

	return &BulkParquetWriter{
		file:          file,
		writer:        writer,
		rowGroupSize:  opts.RowGroupSize,
		rowsInGroup:   0,
	}, nil
}

func (w *BulkParquetWriter) Write(ctx context.Context, key *resourcepb.ResourceKey, value []byte, labels string) error {
	if w.writer == nil {
		return fmt.Errorf("writer is closed")
	}

	actionVal := int32(0)
	row := []parquet.Value{
		{ByteArray: []byte(key.Namespace)},
		{ByteArray: []byte(key.Group)},
		{ByteArray: []byte(key.Resource)},
		{ByteArray: []byte(key.Name)},
		{ByteArray: value},
		{ByteArray: []byte("")},
		{Int32: actionVal},
		{Int64: 0},
		{ByteArray: []byte(labels)},
	}

	if err := w.writer.AddRow(row); err != nil {
		return fmt.Errorf("failed to add row: %w", err)
	}

	w.rowsInGroup++
	return nil
}

func (w *BulkParquetWriter) Close() error {
	if w.writer == nil {
		return nil
	}

	if err := w.writer.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	if err := w.file.Close(); err != nil {
		return fmt.Errorf("failed to close file: %w", err)
	}

	w.writer = nil
	w.file = nil
	return nil
}

func (w *BulkParquetWriter) CloseWithResults() (*resourcepb.BulkResponse, error) {
	if err := w.Close(); err != nil {
		return &resourcepb.BulkResponse{
			Error: &resourcepb.ErrorResult{Message: err.Error()},
		}, err
	}

	return &resourcepb.BulkResponse{
		Processed: int64(w.rowsInGroup),
	}, nil
}

func ReadParquetWithLabels(ctx context.Context, inputPath string, filters []LabelFilter, batchSize int64) (BulkRequestIterator, error) {
	cfg := ParquetReaderConfig{
		InputPath:     inputPath,
		BatchSize:     batchSize,
		LabelFilters:  filters,
	}

	return newParquetReader(ctx, cfg)
}

func CreateMixedDataSourceReader(ctx context.Context, paths []string, filters []LabelFilter, batchSize int64) (*MixedParquetReader, error) {
	readers := make([]BulkRequestIterator, 0, len(paths))

	for _, path := range paths {
		iter, err := ReadParquetWithLabels(ctx, path, filters, batchSize)
		if err != nil {
			for _, r := range readers {
				if closer, ok := r.(io.Closer); ok {
					_ = closer.Close()
				}
			}
			return nil, fmt.Errorf("failed to create reader for %s: %w", path, err)
		}
		readers = append(readers, iter)
	}

	return &MixedParquetReader{
		readers:     readers,
		currentIdx:  0,
	}, nil
}

type MixedParquetReader struct {
	readers    []BulkRequestIterator
	currentIdx int
	currentReq *resourcepb.BulkRequest
	err        error
}

var _ BulkRequestIterator = (*MixedParquetReader)(nil)

func (m *MixedParquetReader) Next() bool {
	m.currentReq = nil

	for m.currentIdx < len(m.readers) {
		reader := m.readers[m.currentIdx]
		if reader.Next() {
			m.currentReq = reader.Request()
			if reader.RollbackRequested() {
				m.err = fmt.Errorf("rollback requested from reader %d", m.currentIdx)
				return false
			}
			return true
		}

		if reader.RollbackRequested() {
			m.err = fmt.Errorf("rollback requested from reader %d", m.currentIdx)
			return false
		}

		m.currentIdx++
	}

	return false
}

func (m *MixedParquetReader) Request() *resourcepb.BulkRequest {
	return m.currentReq
}

func (m *MixedParquetReader) RollbackRequested() bool {
	return m.err != nil
}

func (m *MixedParquetReader) Close() error {
	var lastErr error
	for _, reader := range m.readers {
		if closer, ok := reader.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}