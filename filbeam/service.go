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
)

// Service is a client for the FilBeam stats API.
// It is safe for concurrent use.
type Service struct {
	c          chain.Chain
	httpClient *http.Client
	logger     *slog.Logger
}

// Option customises a Service.
type Option func(*Service)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(s *Service) { s.httpClient = c }
}

// WithLogger sets the structured logger. If nil, logging is silent.
func WithLogger(l *slog.Logger) Option {
	return func(s *Service) { s.logger = l }
}

// NewService creates a Service for the given chain.
func NewService(c chain.Chain, opts ...Option) *Service {
	s := &Service{
		c:          c,
		httpClient: http.DefaultClient,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// baseURL returns the stats API base URL for the configured chain.
// Only chain.Calibration and chain.Mainnet are supported; any unrecognized
// chain falls back to the mainnet endpoint.
func (s *Service) baseURL() string {
	if s.c == chain.Calibration {
		return "https://calibration.stats.filbeam.com"
	}
	return "https://stats.filbeam.com"
}

// statsResponse mirrors the JSON returned by the FilBeam stats API.
// The quota fields are string-encoded integers per the API contract.
type statsResponse struct {
	CDNEgressQuota       string `json:"cdnEgressQuota"`
	CacheMissEgressQuota string `json:"cacheMissEgressQuota"`
}

// GetDataSetStats fetches remaining egress quotas for a FWSS data set.
// Returns ErrDataSetNotFound when the data set does not exist on FilBeam.
func (s *Service) GetDataSetStats(ctx context.Context, dataSetID *big.Int) (*DataSetStats, error) {
	if dataSetID == nil {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: nil dataSetID")
	}

	url := fmt.Sprintf("%s/data-set/%s", s.baseURL(), dataSetID.String())

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
		return nil, fmt.Errorf("filbeam.GetDataSetStats: %w: id=%s", ErrDataSetNotFound, dataSetID)
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
