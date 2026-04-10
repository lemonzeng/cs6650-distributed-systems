package worker

import "io"

// PhotoJob is the unit of work passed from the HTTP handler to the background worker.
// Body is the multipart file handle returned by r.FormFile — the worker owns it and
// must close it after use. For in-memory parts (< 32 MB threshold) Go uses a
// sectionReadCloser whose embedded io.Closer is nil; the worker closes it safely.
// Done, if non-nil, is called after the S3 upload finishes to release the handler's
// semaphore slot (bounding total in-flight memory usage).
type PhotoJob struct {
	PhotoID     string
	AlbumID     string
	Body        io.ReadCloser
	Size        int64
	ContentType string
	Done        func() // release semaphore slot; nil-safe
}

// JobChan is the shared channel type used across the handler and worker packages.
// Buffered at startup (recommended size: 100).
type JobChan chan PhotoJob
