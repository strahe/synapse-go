package filbeam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/internal/retry"
	"github.com/strahe/synapse-go/types"
)

// Service is a client for the FilBeam stats API.
// It is safe for concurrent use.
type Service struct {
	baseURL    string
	httpClient *http.Client
	logger     *slog.Logger
	lifecycle  *lifecycle.Lifecycle
}

// Options configures a [Service].
type Options struct {
	// Chain selects the FilBeam environment. Only chain.Mainnet and
	// chain.Calibration are supported; any other value causes New to
	// return an error wrapping chain.ErrUnknownChain.
	// Zero value is chain.Mainnet.
	Chain chain.Chain

	// HTTPClient is the HTTP client used for API requests.
	// If nil, http.DefaultClient is used.
	HTTPClient *http.Client

	// Logger is the structured logger. If nil, logging is silent.
	Logger *slog.Logger

	// Lifecycle, when non-nil, ties this Service to the owning Client's
	// close state. After the Lifecycle is closed, every method returns
	// ErrClosed. Nil is allowed for standalone use.
	Lifecycle *lifecycle.Lifecycle
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
		lifecycle:  opts.Lifecycle,
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
//
// Transient failures (most transport errors and HTTP 5xx) are retried with
// jittered exponential backoff. Errors matching
// [context.Canceled] or [context.DeadlineExceeded] are returned immediately;
// non-transient statuses (4xx other than 404) and decode errors are also
// returned without retry.
func (s *Service) GetDataSetStats(ctx context.Context, dataSetID types.BigInt) (*DataSetStats, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: %w", ErrInvalidArgument)
	}
	url := fmt.Sprintf("%s/data-set/%s", s.baseURL, dataSetID.String())

	stats, err := retry.Do(ctx, func(ctx context.Context) (*DataSetStats, error) {
		return s.fetchStats(ctx, url, dataSetID)
	}, retry.WithRetryIf(isTransientFilbeamErr))
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// errTransientFilbeam marks fetchStats errors that should be retried. It is
// never returned to callers.
var errTransientFilbeam = errors.New("filbeam: transient")

func isTransientFilbeamErr(err error) bool { return errors.Is(err, errTransientFilbeam) }

func (s *Service) fetchStats(ctx context.Context, url string, dataSetID types.BigInt) (*DataSetStats, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("filbeam.GetDataSetStats: http: %w: %w", errTransientFilbeam, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("filbeam.GetDataSetStats: %w: id=%s", ErrDataSetNotFound, dataSetID.String())
	}
	if resp.StatusCode >= 500 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("filbeam.GetDataSetStats: HTTP %d: %s: %w", resp.StatusCode, string(body), errTransientFilbeam)
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
