package filbeam

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/types"
)

// Service is a client for the FilBeam stats API.
// It is safe for concurrent use.
type Service struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
}

// Options configures a [Service].
type Options struct {
	// Chain selects the FilBeam environment. Only [chain.Mainnet] and
	// [chain.Calibration] are supported; any other value causes [New] to
	// return an error wrapping [chain.ErrUnknownChain].
	// Zero value is [chain.Mainnet].
	Chain chain.Chain

	// HTTPClient is the HTTP client used for API requests.
	// If nil, [http.DefaultClient] is used.
	HTTPClient *http.Client

	// Logger is the structured logger. If nil, logging is silent.
	Logger *slog.Logger
}

// New creates a [Service] for the given chain.
// Returns an error wrapping [chain.ErrUnknownChain] if opts.Chain is not a
// supported FilBeam network.
func New(opts Options) (*Service, error) {
	baseURL, ok := filbeamBaseURL(opts.Chain)
	if !ok {
		return nil, fmt.Errorf("filbeam.New: %w: %v", chain.ErrUnknownChain, opts.Chain)
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Service{
		baseURL:    baseURL,
		httpClient: httpClient,
		logger:     opts.Logger,
	}, nil
}

// filbeamBaseURL returns the stats API base URL for the given chain.
// The second return value reports whether the chain is supported.
func filbeamBaseURL(c chain.Chain) (string, bool) {
	switch c {
	case chain.Mainnet:
		return "https://stats.filbeam.com", true
	case chain.Calibration:
		return "https://calibration.stats.filbeam.com", true
	default:
		return "", false
	}
}

// statsResponse mirrors the JSON returned by the FilBeam stats API.
// The quota fields are string-encoded integers per the API contract.
type statsResponse struct {
	CDNEgressQuota       string `json:"cdnEgressQuota"`
	CacheMissEgressQuota string `json:"cacheMissEgressQuota"`
}

// GetDataSetStats fetches remaining egress quotas for a FWSS data set.
// Returns ErrDataSetNotFound when the data set does not exist on FilBeam.
func (s *Service) GetDataSetStats(ctx context.Context, dataSetID types.DataSetID) (*DataSetStats, error) {
	if dataSetID == 0 {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: %w", ErrInvalidArgument)
	}
	url := fmt.Sprintf("%s/data-set/%d", s.baseURL, uint64(dataSetID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("filbeam.GetDataSetStats: %w: id=%d", ErrDataSetNotFound, uint64(dataSetID))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("filbeam.GetDataSetStats: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw statsResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: decode: %w", err)
	}

	cdnQuota, ok := new(big.Int).SetString(raw.CDNEgressQuota, 10)
	if !ok {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: invalid cdnEgressQuota: %q", raw.CDNEgressQuota)
	}
	cacheMissQuota, ok := new(big.Int).SetString(raw.CacheMissEgressQuota, 10)
	if !ok {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: invalid cacheMissEgressQuota: %q", raw.CacheMissEgressQuota)
	}

	return &DataSetStats{
		CDNEgressQuota:       cdnQuota,
		CacheMissEgressQuota: cacheMissQuota,
	}, nil
}
