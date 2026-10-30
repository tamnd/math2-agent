package cluster

import "io"

// A Sourcer is an ingest adapter that also gives source records (RS13.1). Records reads the same snapshot as the Read method of the adapter and calls emit for each source record. The caller sets the fields Source and Snapshot.
type Sourcer interface {
	Records(snapshot io.Reader, emit func(Record) error) error
}

// SourceFile is the name of the file of the source records of one source in its directory under RecordsDir. Each import writes the file again, so the directory holds the records of one snapshot.
const SourceFile = "records"
