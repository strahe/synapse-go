package chain

// Binary size constants.
const (
	KiB = 1024
	MiB = 1 << 20
	GiB = 1 << 30
	TiB = 1 << 40
	PiB = 1 << 50
)

// Upload size limits and leaf geometry.
const (
	// MaxUploadSize is the maximum data size for a single upload,
	// accounting for fr32 padding (127 usable bytes per 128 bytes).
	MaxUploadSize = GiB * 127 / 128

	// MinUploadSize is the minimum data size for a single upload.
	MinUploadSize = 127

	// BytesPerLeaf is the number of bytes per leaf in a Merkle tree.
	BytesPerLeaf = 32
)
