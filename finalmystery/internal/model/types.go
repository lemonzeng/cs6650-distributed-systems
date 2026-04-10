package model

// Album represents a photo album.
type Album struct {
	AlbumID     string `json:"album_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Owner       string `json:"owner"`
}

// PhotoStatus is the lifecycle state of a photo.
type PhotoStatus string

const (
	StatusProcessing PhotoStatus = "processing"
	StatusCompleted  PhotoStatus = "completed"
	StatusFailed     PhotoStatus = "failed"
	StatusDeleted    PhotoStatus = "deleted" // internal only — never returned to client
)

// Photo represents a photo record.
// URL is omitted from JSON when empty (only present when status=completed).
// S3Key is never serialized — internal use only.
type Photo struct {
	PhotoID string      `json:"photo_id"`
	AlbumID string      `json:"album_id"`
	Seq     int         `json:"seq"`
	Status  PhotoStatus `json:"status"`
	URL     string      `json:"url,omitempty"`
	S3Key   string      `json:"-"`
}
