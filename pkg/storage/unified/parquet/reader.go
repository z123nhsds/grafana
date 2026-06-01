package parquet

import (
	"fmt"

	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"

	"github.com/grafana/grafana/pkg/storage/unified/resource"
	"github.com/grafana/grafana/pkg/storage/unified/resourcepb"
)

var (
	_               resource.BulkRequestIterator = (*parquetReader)(nil)
	openParquetFile                             = file.OpenParquetFile
)

func NewParquetReader(inputPath string, batchSize int64) (resource.BulkRequestIterator, error) {
	return newResourceReader(inputPath, batchSize)
}

type parquetReader struct {
	reader *file.Reader

	namespace *stringColumn
	group     *stringColumn
	resource  *stringColumn
	name      *stringColumn
	value     *stringColumn
	folder    *stringColumn
	action    *int32Column
	columns   []columnBuffer

	batchSize int64

	defLevels []int16
	repLevels []int16

	bufferSize  int
	bufferIndex int
	rowGroupIDX int

	req *resourcepb.BulkRequest
	err error
}

func (r *parquetReader) Next() bool {
	r.req = nil
	for r.err == nil && r.reader != nil {
		if r.bufferIndex >= r.bufferSize && r.value.reader.HasNext() {
			r.err = r.readBulk()
			if r.err != nil {
				r.close()
				return false
			}
		}

		if r.bufferSize > r.bufferIndex {
			i := r.bufferIndex
			r.bufferIndex++

			r.req = &resourcepb.BulkRequest{
				Key: &resourcepb.ResourceKey{
					Group:     r.group.buffer[i].String(),
					Resource:  r.resource.buffer[i].String(),
					Namespace: r.namespace.buffer[i].String(),
					Name:      r.name.buffer[i].String(),
				},
				Action: resourcepb.BulkRequest_Action(r.action.buffer[i]),
				Value:  r.value.buffer[i].Bytes(),
				Folder: r.folder.buffer[i].String(),
			}

			return true
		}

		r.rowGroupIDX++
		if r.rowGroupIDX >= r.reader.NumRowGroups() {
			r.close()
			return false
		}
		r.err = r.open(r.reader.RowGroup(r.rowGroupIDX))
		if r.err != nil {
			r.close()
		}
	}

	return false
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

func newResourceReader(inputPath string, batchSize int64) (*parquetReader, error) {
	rdr, err := openParquetFile(inputPath, false)
	if err != nil {
		return nil, err
	}

	schema := rdr.MetaData().Schema
	makeColumn := func(name string) *stringColumn {
		index := schema.ColumnIndexByName(name)
		if index < 0 {
			err = fmt.Errorf("missing column: %s", name)
		}
		return &stringColumn{
			index:  index,
			buffer: make([]parquet.ByteArray, batchSize),
		}
	}

	reader := &parquetReader{
		reader: rdr,

		namespace: makeColumn("namespace"),
		group:     makeColumn("group"),
		resource:  makeColumn("resource"),
		name:      makeColumn("name"),
		value:     makeColumn("value"),
		folder:    makeColumn("folder"),

		action: &int32Column{
			index:  schema.ColumnIndexByName("action"),
			buffer: make([]int32, batchSize),
		},

		batchSize: batchSize,
		defLevels: make([]int16, batchSize),
		repLevels: make([]int16, batchSize),
	}

	if err != nil {
		_ = rdr.Close()
		return nil, err
	}

	reader.columns = []columnBuffer{
		reader.namespace,
		reader.group,
		reader.resource,
		reader.name,
		reader.folder,
		reader.action,
		reader.value,
	}

	if rdr.NumRowGroups() < 1 {
		err = rdr.Close()
		reader.reader = nil
		return reader, err
	}

	err = reader.open(rdr.RowGroup(0))
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

func (r *parquetReader) open(rgr *file.RowGroupReader) error {
	for _, c := range r.columns {
		err := c.open(rgr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *parquetReader) readBulk() error {
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

type columnBuffer interface {
	open(rgr *file.RowGroupReader) error
	batch(batchSize int64, defLevels []int16, repLevels []int16) (int, error)
}

type stringColumn struct {
	index  int
	reader *file.ByteArrayColumnChunkReader
	buffer []parquet.ByteArray
	count  int
}

func (c *stringColumn) open(rgr *file.RowGroupReader) error {
	tmp, err := rgr.Column(c.index)
	if err != nil {
		return err
	}
	var ok bool
	c.reader, ok = tmp.(*file.ByteArrayColumnChunkReader)
	if !ok {
		return fmt.Errorf("expected resource strings")
	}
	return nil
}

func (c *stringColumn) batch(batchSize int64, defLevels []int16, repLevels []int16) (int, error) {
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
	tmp, err := rgr.Column(c.index)
	if err != nil {
		return err
	}
	var ok bool
	c.reader, ok = tmp.(*file.Int32ColumnChunkReader)
	if !ok {
		return fmt.Errorf("expected resource strings")
	}
	return nil
}

func (c *int32Column) batch(batchSize int64, defLevels []int16, repLevels []int16) (int, error) {
	_, count, err := c.reader.ReadBatch(batchSize, c.buffer, defLevels, repLevels)
	c.count = count
	return count, err
}
